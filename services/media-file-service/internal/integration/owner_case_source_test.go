package integration_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	mediahttp "cosmicforge/logistics/services/media-file-service/internal/features/mediaobjects/http"
	"cosmicforge/logistics/services/media-file-service/internal/features/mediaobjects/usecases"
	"cosmicforge/logistics/shared/go/admincase"
	"cosmicforge/logistics/shared/go/admincontext"
	contextfixture "cosmicforge/logistics/shared/go/admincontext/testfixture"
	"cosmicforge/logistics/shared/go/adminmedia"
	"cosmicforge/logistics/shared/go/adminowner"
	"cosmicforge/logistics/shared/go/adminpolicy"
	policyfixture "cosmicforge/logistics/shared/go/adminpolicy/testfixture"
	"cosmicforge/logistics/shared/go/adminread"
	"cosmicforge/logistics/shared/go/customerreview"
	"cosmicforge/logistics/shared/go/serviceauth"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Real owner executable, real owner migrations and review commands, independent
// TEST_ONLY owner signatures, modern Media upload, native HTTP and both SDKs.
// No CaseAuthority stub or legacy-ID mapping participates in this path.
func TestPostgresMediaNativeCustomerCaseMembershipAndModernEvidence(t *testing.T) {
	f := newMediaFixture(t, fixtureOptions{scanner: cleanScanner{}})
	ctx := context.Background()
	scope := adminpolicy.Scope{Product: "karryam", Market: "M1", Zone: "Z1", Organization: testOrgID, Tenant: testOrgID}
	spec, _ := admincase.Find("customer-service", "review-work-items")
	policy := policyfixture.New(t, scope, spec.Owner, spec.Permission, spec.SourceContract)
	policy.Document.ApprovedOwners = append(policy.Document.ApprovedOwners, "media-file-service")
	for _, pair := range [][3]string{{adminmedia.Permission, adminmedia.Contract, "media-file-service"}, {customerreview.RequestCapability, customerreview.Contract, spec.Owner}, {customerreview.ReadCapability, customerreview.StateContract, spec.Owner}, {customerreview.WorkItemCapability, customerreview.WorkItemContract, spec.Owner}, {customerreview.WorkItemReadCapability, customerreview.WorkItemContract, spec.Owner}} {
		rule := policy.Document.Rules[0]
		rule.Capability = pair[0]
		rule.Permission = pair[0]
		rule.SourceContract = pair[1]
		rule.Owner = pair[2]
		policy.Document.Rules = append(policy.Document.Rules, rule)
	}
	policy.Document.Media = &adminpolicy.MediaDisclosure{MaxGrantSeconds: 60, SourceCapabilities: []string{spec.Permission}} // TEST_ONLY, never installed in a running environment.
	policy.Sign(t)
	issuer := contextfixture.New(t, spec.Owner)
	origin, sourcePool, keys := startCustomerCaseOwner(t, f, policy, issuer)
	customer := uuid.NewString()
	if _, err := sourcePool.Exec(ctx, `INSERT INTO customers(id,phone)VALUES($1,$2)`, customer, "+399"+strings.ReplaceAll(customer, "-", "")); err != nil {
		t.Fatal(err)
	}
	ownerBase := "/internal/customer/v1/admin"
	prepare := func(base, kind, ref, key string) admincontext.RecordBinding {
		raw := ownerCaseCall(t, "GET", origin+base+"/authorization-contexts/bindings/"+kind+"/"+ref+"?purpose=case-1", nil, keys[key], 200)
		var out struct {
			State   string                     `json:"state"`
			Binding admincontext.RecordBinding `json:"binding"`
		}
		if json.Unmarshal(raw, &out) != nil || out.State != "DRAFT_PENDING_OWNER_APPROVAL" {
			t.Fatalf("invalid native binding: %s", raw)
		}
		return out.Binding
	}
	publish := func(base, key string, a admincontext.Approval) {
		ownerCaseCall(t, "POST", origin+base+"/authorization-contexts", issuer.Sign(t, a), keys[key], 200)
	}
	allocation := issuer.Approval("review-allocation", "review-states", customer, customerreview.RequestCapability, prepare(ownerBase, "review-states", customer, "publisher"), scope)
	allocation.Capabilities = append(allocation.Capabilities, customerreview.ReadCapability, customerreview.WorkItemCapability, customerreview.WorkItemReadCapability)
	publish(ownerBase, "publisher", allocation)
	q := adminread.Query{Scope: adminread.Scope{Market: scope.Market, Zone: scope.Zone, Organization: scope.Organization, Tenant: scope.Tenant}, Staff: "staff-1", Role: "staff", Purpose: "case-1", PolicyID: policy.Engine.ID(), PolicyVersion: policy.Engine.Version(), PolicyChecksum: policy.Engine.Checksum(), Start: time.Now().Add(-time.Hour), End: time.Now(), Limit: 1}
	reviews, err := customerreview.NewClient(origin, keys["write"], keys["readback"])
	if err != nil {
		t.Fatal(err)
	}
	defer reviews.Close()
	receipt, err := reviews.Request(ctx, q, customerreview.Request{ContractVersion: customerreview.Contract, RequestID: uuid.NewString(), CustomerID: customer, ExpectedReviewRevision: 1, IdempotencyKey: "case-review", ReasonCode: "SUPPORT_REVIEW", RequestedAt: time.Now().UTC()})
	if err != nil {
		t.Fatal("native intake", err)
	}
	caseBase := ownerBase + "/case-authority"
	caseAllocation := issuer.Approval("case-allocation", admincase.ContextPrefix+spec.Kind, receipt.ReviewRequestID, spec.Permission, prepare(caseBase, admincase.ContextPrefix+spec.Kind, receipt.ReviewRequestID, "case-publisher"), scope)
	caseAllocation.ExpiresAt = time.Now().UTC().Add(45 * time.Second)
	publish(caseBase, "case-publisher", caseAllocation)
	source, err := admincase.NewClient(spec.Owner, origin, keys["case-source"])
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	authority := &usecases.OwnerCaseAuthority{Media: f.service, Policy: policy.Engine, Sources: map[string]*admincase.Client{spec.Owner: source}}
	// A genuine modern object is issued by its owning workload and bound to the
	// actual review case. A supplied legacy UUID cannot establish this relation.
	payload := pdfFile(0)
	caller := usecases.Caller{Service: spec.Owner, OrganizationID: testOrgID}
	upload, err := f.service.AuthorizeUpload(ctx, usecases.AuthorizeUploadInput{Caller: caller, OwnerType: "CUSTOMER", OwnerReference: customer, UploadPurpose: "COMPLIANCE_DOCUMENT", Visibility: "PRIVATE", FileName: "review.pdf", AllowedMIMETypes: []string{"application/pdf"}, MaxSizeBytes: 1024, RetentionClass: "COMPLIANCE", IdempotencyKey: "modern-case-upload", RequestFingerprint: "native-case-upload", BindingType: spec.BindingType, BindingID: receipt.ReviewRequestID})
	if err != nil {
		t.Fatal("modern upload", err)
	}
	_, err = f.service.CompleteUpload(ctx, usecases.CompleteUploadInput{Caller: caller, MediaObjectID: upload.MediaObjectID, AuthorizationID: upload.AuthorizationID, UploadToken: upload.UploadToken, DeclaredMIME: "application/pdf", DeclaredSize: int64(len(payload)), Content: payload})
	if err != nil {
		t.Fatal("modern completion", err)
	}
	access := f.service.NewCaseAccess(usecases.CaseAccessConfig{Enabled: true, Policy: policy.Engine, Authority: authority})
	router := gin.New()
	mediaKey := bytes.Repeat([]byte("m"), 32)
	mediahttp.NewAdminCaseHandler(access, f.pool, mediaKey, true, true).Register(router)
	server := httptest.NewServer(router)
	defer server.Close()
	client, err := adminmedia.NewClient(server.URL, mediaKey)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	actor := adminmedia.Actor{Product: "karryam", Staff: q.Staff, Roles: []string{"staff"}, Permissions: []string{adminmedia.Permission, spec.Permission}, Markets: []string{scope.Market}, Zones: []string{scope.Zone}, Organizations: []string{scope.Organization}, Tenants: []string{scope.Tenant}, ExpiresAt: time.Now().Add(5 * time.Minute)}
	request := adminmedia.Request{Scope: scope, Purpose: q.Purpose, SourceCapability: spec.Permission, CaseOwner: spec.Owner, CaseKind: spec.Kind, CaseReference: receipt.ReviewRequestID, SubjectType: "CUSTOMER", SubjectReference: customer, MediaObjectID: upload.MediaObjectID}
	grant, err := client.Issue(ctx, actor, request)
	if err != nil {
		t.Fatal("native case grant", err)
	}
	if grant.ExpiresAt.After(caseAllocation.ExpiresAt) {
		t.Fatal("grant outlived independently verified source authorization")
	}
	content, err := client.Read(ctx, actor, request, grant.ID, grant.Token)
	if err != nil || !bytes.Equal(content, payload) {
		t.Fatal("native case content", err)
	}
	for _, name := range []string{"subject", "tenant", "case", "legacy"} {
		t.Run(name, func(t *testing.T) {
			bad := request
			switch name {
			case "subject":
				bad.SubjectReference = uuid.NewString()
			case "tenant":
				bad.Scope.Tenant = "foreign"
			case "case":
				bad.CaseReference = uuid.NewString()
			case "legacy":
				id := uuid.NewString()
				if _, err := f.pool.Exec(ctx, `INSERT INTO media_assets(id,owner_service,owner_id,purpose,content_type,size_bytes,checksum_sha256,storage_bucket,storage_path,public_url)VALUES($1::uuid,$2,$3,'COMPLIANCE_DOCUMENT','application/pdf',1,'legacy','test-only',$4,'https://test.invalid/private')`, id, spec.Owner, customer, "legacy/"+id); err != nil {
					t.Fatal(err)
				}
				bad.MediaObjectID = id
			}
			if _, err := client.Issue(ctx, actor, bad); err == nil {
				t.Fatal("unbound case evidence authorized")
			}
		})
	}
	// Source immutable audit failure blocks both minting and existing redemption.
	if _, err := sourcePool.Exec(ctx, `CREATE FUNCTION fail_case_audit()RETURNS trigger LANGUAGE plpgsql AS $$BEGIN IF NEW.outcome='CASE_SUBJECT_AUTHORIZED' THEN RAISE EXCEPTION 'test audit unavailable';END IF;RETURN NEW;END$$;CREATE TRIGGER fail_case_audit BEFORE INSERT ON admin_owner_read_audit FOR EACH ROW EXECUTE FUNCTION fail_case_audit()`); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Issue(ctx, actor, request); err == nil {
		t.Fatal("grant despite failed owner audit")
	}
	if body, err := client.Read(ctx, actor, request, grant.ID, grant.Token); err == nil || len(body) > 0 {
		t.Fatal("disclosure despite failed owner audit")
	}
	if _, err := sourcePool.Exec(ctx, `DROP TRIGGER fail_case_audit ON admin_owner_read_audit;DROP FUNCTION fail_case_audit()`); err != nil {
		t.Fatal(err)
	}
	caseAllocation.Revision++
	caseAllocation.State = "REVOKED"
	publish(caseBase, "case-publisher", caseAllocation)
	if body, err := client.Read(ctx, actor, request, grant.ID, grant.Token); err == nil || len(body) > 0 {
		t.Fatal("revoked owner allocation served old grant")
	}
	caseAllocation.Revision++
	caseAllocation.State = "APPROVED"
	publish(caseBase, "case-publisher", caseAllocation)
	for i, action := range []string{"ACKNOWLEDGE", "CLOSE"} {
		_, err := reviews.TransitionWorkItem(ctx, q, customerreview.WorkItemCommand{ContractVersion: customerreview.WorkItemContract, OperationID: uuid.NewString(), ReviewRequestID: receipt.ReviewRequestID, CustomerID: customer, ExpectedVersion: int64(i + 1), Action: action, IdempotencyKey: "transition-" + action, ReasonCode: "SUPPORT_REVIEW", RequestedAt: time.Now().UTC()})
		if err != nil {
			t.Fatal("native review transition", err)
		}
		if action == "ACKNOWLEDGE" {
			if body, err := client.Read(ctx, actor, request, grant.ID, grant.Token); err == nil || len(body) > 0 {
				t.Fatal("changed native case revision retained old grant")
			}
			current, err := client.Issue(ctx, actor, request)
			if err != nil {
				t.Fatal("active acknowledged case required invented new source approval", err)
			}
			if body, err := client.Read(ctx, actor, request, current.ID, current.Token); err != nil || !bytes.Equal(body, payload) {
				t.Fatal("acknowledged native case unavailable", err)
			}
		}
	}
	if _, err := client.Issue(ctx, actor, request); err == nil {
		t.Fatal("closed native case minted evidence grant")
	}
	if body, err := client.Read(ctx, actor, request, grant.ID, grant.Token); err == nil || len(body) > 0 {
		t.Fatal("closed native case served old grant")
	}
	if err := client.Revoke(ctx, actor, grant.ID, "CASE_CLOSED"); err != nil {
		t.Fatal("revocation after source closure", err)
	}
	var standing, state string
	if err := sourcePool.QueryRow(ctx, `SELECT c.status,w.status FROM customers c JOIN customer_admin_review_work_items w ON w.customer_id=c.id WHERE w.request_id=$1`, receipt.ReviewRequestID).Scan(&standing, &state); err != nil || standing != "active" || state != "CLOSED" {
		t.Fatal("review changed account standing or failed closure", standing, state, err)
	}
}

func ownerCaseCall(t *testing.T, method, address string, raw, key []byte, want int) []byte {
	t.Helper()
	request, err := http.NewRequest(method, address, bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if method == "POST" {
		request.Header.Set("Content-Type", "application/json")
	}
	if err := serviceauth.SignRequest(request, adminowner.ContextPublisher, key, raw, time.Now()); err != nil {
		t.Fatal(err)
	}
	client := http.Client{Timeout: 5 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 32769))
	if err != nil || response.StatusCode != want {
		t.Fatalf("owner %s %s: status %d body %s error %v", method, request.URL.Path, response.StatusCode, body, err)
	}
	return body
}

func startCustomerCaseOwner(t *testing.T, f *mediaFixture, p *policyfixture.Fixture, issuer *contextfixture.Fixture) (string, *pgxpool.Pool, map[string][]byte) {
	t.Helper()
	ctx := context.Background()
	root, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	binary := filepath.Join(dir, "customer-owner")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	build := exec.Command("go", "build", "-tags", "admin_policy_test", "-o", binary, "./services/customer-service/cmd")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build actual Customer: %v %s", err, out)
	}
	schema := "source_customer_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := f.pool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := f.pool.Exec(context.Background(), `DROP SCHEMA `+schema+` CASCADE`); err != nil {
			t.Error(err)
		}
	})
	dsn, err := url.Parse(os.Getenv("MEDIA_FILE_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	values := dsn.Query()
	values.Set("search_path", schema+",public")
	dsn.RawQuery = values.Encode()
	pool, err := pgxpool.New(ctx, dsn.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := os.Mkdir(filepath.Join(dir, "migrations"), 0700); err != nil {
		t.Fatal(err)
	}
	migrations, err := filepath.Glob(filepath.Join(root, "services/customer-service/migrations/*.sql"))
	if err != nil || len(migrations) == 0 {
		t.Fatal("actual Customer migrations missing", err)
	}
	for _, path := range migrations {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "migrations", filepath.Base(path)), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write := func(name string, raw []byte) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	public := map[string]string{}
	for id, key := range issuer.Authority.Keys {
		public[id] = base64.RawURLEncoding.EncodeToString(key)
	}
	publicRaw, _ := json.Marshal(public)
	env := map[string]string{"APP_ENV": "test", "MIGRATION": "true", "CUSTOMER_DATABASE_URL": dsn.String(), "CUSTOMER_REDIS_ADDR": os.Getenv("MEDIA_CASE_TEST_REDIS_ADDR"), "CUSTOMER_REDIS_DB": "15", "CUSTOMER_REDIS_PASSWORD": "", "CUSTOMER_TEST_LOGIN_PHONES": "", "CUSTOMER_SHARE_JOURNEY_ENABLED": "false", "CUSTOMER_ADMIN_RESOURCE_READ_ENABLED": "true", "CUSTOMER_ADMIN_CONTEXT_PUBLISH_ENABLED": "true", "CUSTOMER_ADMIN_REVIEW_WRITES_ENABLED": "true", "CUSTOMER_ADMIN_REVIEW_READBACK_ENABLED": "true", "CUSTOMER_ADMIN_REVIEW_WORK_ITEMS_ENABLED": "true", "CUSTOMER_ADMIN_CASE_SOURCE_READ_ENABLED": "true", "CUSTOMER_ADMIN_CASE_CONTEXT_PUBLISH_ENABLED": "true", "CUSTOMER_ADMIN_CONTEXT_KEYS_FILE": write("owner-keys.json", publicRaw), "CUSTOMER_ADMIN_ACCESS_POLICY_FILE": write("policy.json", p.Envelope), "CUSTOMER_ADMIN_ACCESS_POLICY_KEYS_FILE": write("policy-keys.json", p.Keys)}
	if env["CUSTOMER_REDIS_ADDR"] == "" {
		env["CUSTOMER_REDIS_ADDR"] = "127.0.0.1:6379"
	}
	keys := map[string][]byte{}
	for i, entry := range [][2]string{{"resource", "CUSTOMER_ADMIN_RESOURCE_SECRET_FILE"}, {"publisher", "CUSTOMER_ADMIN_CONTEXT_PUBLISH_SECRET_FILE"}, {"write", "CUSTOMER_ADMIN_REVIEW_WRITE_SECRET_FILE"}, {"readback", "CUSTOMER_ADMIN_REVIEW_READBACK_SECRET_FILE"}, {"case-source", "CUSTOMER_ADMIN_CASE_SOURCE_SECRET_FILE"}, {"case-publisher", "CUSTOMER_ADMIN_CASE_CONTEXT_PUBLISH_SECRET_FILE"}} {
		key := bytes.Repeat([]byte{byte('a' + i)}, 32)
		keys[entry[0]] = key
		env[entry[1]] = write(entry[0]+".key", []byte(base64.RawURLEncoding.EncodeToString(key)))
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	listener.Close()
	env["HTTP_ADDR"] = address
	logPath := filepath.Join(dir, "owner.log")
	log, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(binary)
	command.Dir = dir
	command.Stdout = log
	command.Stderr = log
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if _, overridden := env[name]; !overridden {
			command.Env = append(command.Env, entry)
		}
	}
	for k, v := range env {
		command.Env = append(command.Env, k+"="+v)
	}
	if err := command.Start(); err != nil {
		log.Close()
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	t.Cleanup(func() {
		_ = command.Process.Kill()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("Customer fixture did not exit")
		}
		log.Close()
	})
	origin := "http://" + address
	client := http.Client{Timeout: time.Second}
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case err := <-done:
			body, _ := os.ReadFile(logPath)
			t.Fatalf("native Customer exited: %v %s", err, body)
		default:
		}
		response, err := client.Get(origin + "/health")
		if err == nil {
			response.Body.Close()
			if response.StatusCode == 200 {
				return origin, pool, keys
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	body, _ := os.ReadFile(logPath)
	t.Fatal(fmt.Sprintf("native Customer not ready: %s", body))
	return "", nil, nil
}
