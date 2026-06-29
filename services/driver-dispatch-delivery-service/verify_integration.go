package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
)

const (
	baseURL   = "http://localhost:8103"
	dbURL     = "postgres://cosmicforge_logistics:cosmicforge_logistics@localhost:5435/dispatch_delivery_service?sslmode=disable"
	redisAddr = "localhost:6382"
	devSecret = "development-dispatch-rider-access-token-secret"
)

type EventMsg struct {
	Topic   string
	Payload string
}

func main() {
	fmt.Println("==================================================")
	fmt.Println("STARTING PHASE 4G-4I RUNTIME INTEGRATION VALIDATION")
	fmt.Println("==================================================")

	// 1. Connect to DB
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dbURL)
	if err != nil {
		fmt.Printf("❌ Failed to connect to Postgres: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close(ctx)
	fmt.Println("✅ Connected to Postgres database.")

	// 2. Connect to Redis
	rdb := redis.NewClient(&redis.Options{
		Addr: redisAddr,
	})
	defer rdb.Close()
	if err := rdb.Ping(ctx).Err(); err != nil {
		fmt.Printf("❌ Failed to connect to Redis: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅ Connected to Redis.")

	// Clear rate limit key for OTP
	rdb.Del(ctx, "dispatch_rider_auth:otp_rate:+2348099001001")
	fmt.Println("🧹 Cleared Redis OTP rate limit key for +2348099001001.")

	// 3. Subscribe to Redis topics in background
	topics := []string{
		"vehicle.registered",
		"vehicle.documents.submitted",
		"vehicle.verified",
		"vehicle.rejected",
		"vehicle.suspended",
	}
	pubsub := rdb.Subscribe(ctx, topics...)
	defer pubsub.Close()

	var eventsMu sync.Mutex
	receivedEvents := make(map[string][]string)

	go func() {
		for {
			msg, err := pubsub.ReceiveMessage(ctx)
			if err != nil {
				return
			}
			eventsMu.Lock()
			receivedEvents[msg.Channel] = append(receivedEvents[msg.Channel], msg.Payload)
			eventsMu.Unlock()
			fmt.Printf("👉 [Redis Event Received] Topic: %s | Payload: %s\n", msg.Channel, msg.Payload)
		}
	}()
	time.Sleep(200 * time.Millisecond) // Allow subscription to activate

	// Helper to sign platform_admin JWT with a valid UUID for reviewerID
	adminToken := makeJWT(devSecret, "00000000-0000-0000-0000-000000000001", "+2348001000099", "admin-session-123", "platform_admin")

	// Step A: Route Protection Check (No Token)
	fmt.Println("\n--- Step A: Route Protection Check (No Token) ---")
	testRouteProtectionNoToken()

	// Step B: OTP generation & Verify Flow
	fmt.Println("\n--- Step B: Start OTP & Verify for Provider ---")
	providerPhone := "+2348099001001"
	otpCode := startOTPAndGetFromLogs(providerPhone)
	if otpCode == "" {
		fmt.Println("❌ Could not retrieve OTP code from logs!")
		os.Exit(1)
	}
	fmt.Printf("🔑 OTP retrieved: %s\n", otpCode)

	tokenRes := verifyOTP(providerPhone, otpCode)
	providerToken := tokenRes.AccessToken
	providerID := tokenRes.ProviderID
	fmt.Printf("👤 Provider authenticated successfully. ID: %s\n", providerID)

	// DB cleanup to ensure idempotency and avoid "plate number already registered" / foreign key conflicts
	_, _ = conn.Exec(ctx, "DELETE FROM bike_documents WHERE provider_id = $1", providerID)
	_, _ = conn.Exec(ctx, "DELETE FROM bike_audit WHERE provider_id = $1", providerID)
	_, _ = conn.Exec(ctx, "DELETE FROM bikes WHERE provider_id = $1", providerID)
	_, _ = conn.Exec(ctx, "DELETE FROM bike_documents WHERE bike_id IN (SELECT id FROM bikes WHERE plate_number IN ('ABC-123-XYZ', 'XYZ-789-ABC'))")
	_, _ = conn.Exec(ctx, "DELETE FROM bike_audit WHERE bike_id IN (SELECT id FROM bikes WHERE plate_number IN ('ABC-123-XYZ', 'XYZ-789-ABC'))")
	_, _ = conn.Exec(ctx, "DELETE FROM bikes WHERE plate_number IN ('ABC-123-XYZ', 'XYZ-789-ABC')")
	fmt.Println("🧹 DB cleanup complete: Cleared existing bikes, documents, and audits.")

	// Step C: Route Protection Check (Invalid Role on Admin review)
	fmt.Println("\n--- Step C: Route Protection Check (Provider JWT on Admin reviewer route) ---")
	testRouteProtectionProviderOnAdmin(providerToken)

	// Step D: Register Bike (Phase 4C + 4I vehicle.registered publication)
	fmt.Println("\n--- Step D: Registering Bike ---")
	bikeID := registerBike(providerToken)
	fmt.Printf("🚲 Registered bike ID: %s\n", bikeID)

	// Verify vehicle.registered event
	time.Sleep(500 * time.Millisecond)
	verifyEventReceived(&eventsMu, &receivedEvents, "vehicle.registered")

	// Step E: GET /provider/vehicle/:id/documents (Empty list)
	fmt.Println("\n--- Step E: GET Documents (Empty list) ---")
	getDocumentsEmpty(providerToken, bikeID)

	// Step F: Upload Document (Phase 4F + 4I vehicle.documents.submitted publication)
	fmt.Println("\n--- Step F: Uploading Document ---")
	uploadDocument(providerToken, bikeID)

	// Verify vehicle.documents.submitted event
	time.Sleep(500 * time.Millisecond)
	verifyEventReceived(&eventsMu, &receivedEvents, "vehicle.documents.submitted")

	// Step G: GET /provider/vehicle/:id/documents (Not empty + local-private schema check)
	fmt.Println("\n--- Step G: GET Documents (Verified local-private schema) ---")
	getDocumentsWithContent(providerToken, bikeID, providerID)

	// Step H: Admin Review Approved (Phase 4H + 4I vehicle.verified + DB Audit check)
	fmt.Println("\n--- Step H: Admin Review - APPROVED ---")
	adminReview(adminToken, bikeID, "approved", "")
	time.Sleep(500 * time.Millisecond)
	verifyEventReceived(&eventsMu, &receivedEvents, "vehicle.verified")
	verifyBikeDBState(ctx, conn, bikeID, "verified", true)
	verifyAuditRow(ctx, conn, bikeID, "approved", "")

	// Step I: Conflict check - Approved again
	fmt.Println("\n--- Step I: Admin Review Conflict Check (Approve already verified) ---")
	adminReviewExpectConflict(adminToken, bikeID, "approved", "")

	// Step J: Admin Review Suspended (Phase 4H + 4I vehicle.suspended + DB Audit check)
	fmt.Println("\n--- Step J: Admin Review - SUSPENDED ---")
	adminReview(adminToken, bikeID, "suspended", "Suspended due to validation check")
	time.Sleep(500 * time.Millisecond)
	verifyEventReceived(&eventsMu, &receivedEvents, "vehicle.suspended")
	verifyBikeDBState(ctx, conn, bikeID, "suspended", false)
	verifyAuditRow(ctx, conn, bikeID, "suspended", "Suspended due to validation check")

	// Step K: Conflict check - Suspend again
	fmt.Println("\n--- Step K: Admin Review Conflict Check (Suspend already suspended) ---")
	adminReviewExpectConflict(adminToken, bikeID, "suspended", "Suspended again")

	// Step L: Test provider.verification.suspended subscriber (bulk suspend + audit rows per bike)
	fmt.Println("\n--- Step L: provider.verification.suspended Subscriber ---")
	// Let's register a second bike for the provider to test bulk suspension
	bikeID2 := registerBikeWithPlate(providerToken, "XYZ-789-ABC")
	fmt.Printf("🚲 Second bike registered. ID: %s\n", bikeID2)

	// Verify bike 2 is initially active (false/true depending on registration rules, but is unverified)
	verifyBikeDBState(ctx, conn, bikeID2, "unverified", true)

	// Publish provider.verification.suspended event
	pubEvent := map[string]any{
		"event":          "provider.verification.suspended",
		"correlation_id": "suspend-corr-999",
		"provider_id":    providerID,
		"reason":         "Entire provider account suspended",
		"created_at":     time.Now().Format(time.RFC3339),
	}
	pubBytes, _ := json.Marshal(pubEvent)
	fmt.Printf("📣 Publishing provider.verification.suspended message for provider_id=%s\n", providerID)
	rdb.Publish(ctx, "provider.verification.suspended", pubBytes)

	// Wait for subscriber execution
	time.Sleep(1500 * time.Millisecond)

	// Both bikes should now be suspended and inactive
	verifyBikeDBState(ctx, conn, bikeID, "suspended", false)
	verifyBikeDBState(ctx, conn, bikeID2, "suspended", false)

	// Check audit rows for bulk suspension
	verifyAuditRow(ctx, conn, bikeID2, "suspended", "Entire provider account suspended")
	fmt.Println("✅ provider.verification.suspended subscriber successfully processed bulk suspension.")

	// Step M: Subscriber Bad Payload safety
	fmt.Println("\n--- Step M: Subscriber Bad Payload safety ---")
	fmt.Println("📣 Publishing malformed payload to provider.verification.suspended topic...")
	rdb.Publish(ctx, "provider.verification.suspended", `{"provider_id": "123", "correlation_id"`) // broken JSON

	time.Sleep(1000 * time.Millisecond)
	// Check ready endpoint
	resp, err := http.Get(baseURL + "/ready")
	if err != nil || resp.StatusCode != http.StatusOK {
		fmt.Println("❌ Service crashed on bad subscriber payload!")
		os.Exit(1)
	}
	fmt.Println("✅ Service is still running. Bad payload was safely caught and dropped without crashing.")

	fmt.Println("\n==================================================")
	fmt.Println("ALL INTEGRATION TESTS AND RUNTIME CLOSURE CRITERIA PASSED!")
	fmt.Println("==================================================")
}

func makeJWT(secret, dispatchRiderID, phone, sessionID, role string) string {
	now := time.Now().UTC()
	exp := now.Add(24 * time.Hour)

	header := map[string]string{"alg": "HS256", "typ": "JWT"}
	claims := map[string]any{
		"dispatch_rider_id": dispatchRiderID,
		"phone_number":      phone,
		"session_id":        sessionID,
		"role":              role,
		"token_type":        "access",
		"iat":               now.Unix(),
		"exp":               exp.Unix(),
	}

	hj, _ := json.Marshal(header)
	cj, _ := json.Marshal(claims)

	unsigned := base64.RawURLEncoding.EncodeToString(hj) + "." + base64.RawURLEncoding.EncodeToString(cj)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(unsigned))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	return unsigned + "." + signature
}

func startOTPAndGetFromLogs(phone string) string {
	fmt.Printf("POST /api/v1/auth/start for %s\n", phone)
	reqBody, _ := json.Marshal(map[string]string{"phone_number": phone})
	resp, err := http.Post(baseURL+"/api/v1/auth/start", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		fmt.Printf("❌ Failed to start OTP: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("❌ Unexpected status code: %d | Body: %s\n", resp.StatusCode, string(body))
		os.Exit(1)
	}

	// Fetch logs from docker compose
	time.Sleep(1 * time.Second)
	cmd := exec.Command("docker", "compose", "-f", "../../infra/docker-compose.yml", "logs", "--tail=15", "driver-dispatch-delivery-service")
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Try without relative path if running from service directory
		cmd = exec.Command("docker", "compose", "-f", "infra/docker-compose.yml", "logs", "--tail=15", "driver-dispatch-delivery-service")
		output, err = cmd.CombinedOutput()
		if err != nil {
			fmt.Printf("❌ Error running docker compose logs: %v\n", err)
			return ""
		}
	}

	re := regexp.MustCompile(`otp=(\d{6})`)
	matches := re.FindAllStringSubmatch(string(output), -1)
	if len(matches) > 0 {
		// return latest OTP
		return matches[len(matches)-1][1]
	}
	return ""
}

type TokenResult struct {
	ProviderID  string `json:"provider_id"`
	AccessToken string `json:"access_token"`
}

type APIResponse struct {
	Success bool        `json:"success"`
	Data    TokenResult `json:"data"`
}

func verifyOTP(phone, code string) TokenResult {
	fmt.Printf("POST /api/v1/auth/verify code=%s\n", code)
	reqBody, _ := json.Marshal(map[string]string{
		"phone_number": phone,
		"otp_code":     code,
	})
	resp, err := http.Post(baseURL+"/api/v1/auth/verify", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		fmt.Printf("❌ Failed to verify OTP: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		fmt.Printf("❌ Unexpected verification status: %d | Body: %s\n", resp.StatusCode, string(body))
		os.Exit(1)
	}

	var res APIResponse
	if err := json.Unmarshal(body, &res); err != nil {
		fmt.Printf("❌ Failed to decode token response: %v\n", err)
		os.Exit(1)
	}
	return res.Data
}

func testRouteProtectionNoToken() {
	routes := []string{
		"GET /api/v1/provider/vehicle",
		"POST /api/v1/provider/vehicle",
		"PATCH /api/v1/admin/vehicle/11111111-1111-1111-1111-111111111111/review",
	}

	for _, route := range routes {
		parts := strings.Split(route, " ")
		method, path := parts[0], parts[1]

		req, _ := http.NewRequest(method, baseURL+path, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			fmt.Printf("❌ Failed request on route protection: %v\n", err)
			os.Exit(1)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusUnauthorized {
			fmt.Printf("❌ Route protection failed for %s. Expected 401, got %d\n", route, resp.StatusCode)
			os.Exit(1)
		}
		fmt.Printf("✅ Route %s properly returned 401 Unauthorized.\n", route)
	}
}

func testRouteProtectionProviderOnAdmin(token string) {
	req, _ := http.NewRequest("PATCH", baseURL+"/api/v1/admin/vehicle/11111111-1111-1111-1111-111111111111/review", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Printf("❌ Request error: %v\n", err)
		os.Exit(1)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		fmt.Printf("❌ Route protection failed. Provider JWT on admin route got %d, expected 403\n", resp.StatusCode)
		os.Exit(1)
	}
	fmt.Println("✅ Route PATCH /api/v1/admin/vehicle/:id/review properly returned 403 Forbidden for provider JWT.")
}

func registerBike(token string) string {
	return registerBikeWithPlate(token, "ABC-123-XYZ")
}

func registerBikeWithPlate(token, plate string) string {
	body := map[string]any{
		"bike_type":    "dispatch_bike",
		"brand":        "Honda",
		"model":        "CG-125",
		"year":         2022,
		"color":        "Red",
		"plate_number": plate,
	}
	reqBody, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/provider/vehicle", bytes.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Printf("❌ Request error: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		fmt.Printf("❌ Register bike failed status=%d body=%s\n", resp.StatusCode, string(bodyBytes))
		os.Exit(1)
	}

	var parsed map[string]any
	json.Unmarshal(bodyBytes, &parsed)
	data := parsed["data"].(map[string]any)
	return data["id"].(string)
}

func getDocumentsEmpty(token, bikeID string) {
	req, _ := http.NewRequest("GET", baseURL+"/api/v1/provider/vehicle/"+bikeID+"/documents", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Printf("❌ Request error: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		fmt.Printf("❌ GET documents failed status=%d body=%s\n", resp.StatusCode, string(bodyBytes))
		os.Exit(1)
	}

	var parsed map[string]any
	json.Unmarshal(bodyBytes, &parsed)
	docs := parsed["data"].([]any)
	if len(docs) != 0 {
		fmt.Printf("❌ Expected 0 documents for new bike, got %d\n", len(docs))
		os.Exit(1)
	}
	fmt.Println("✅ GET documents returned empty array [] for new bike.")
}

func uploadDocument(token, bikeID string) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Document Type
	_ = writer.WriteField("document_type", "insurance")

	// Expiry date for insurance
	_ = writer.WriteField("expiry_date", "2028-12-31")

	// File - name field must be "document_file"
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", `form-data; name="document_file"; filename="insurance.png"`)
	h.Set("Content-Type", "image/png")
	part, _ := writer.CreatePart(h)
	part.Write([]byte("\x89PNG\r\n\x1a\nfake png file content bytes"))
	writer.Close()

	req, _ := http.NewRequest("POST", baseURL+"/api/v1/provider/vehicle/"+bikeID+"/documents", body)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Printf("❌ Request error: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		fmt.Printf("❌ Upload document failed status=%d body=%s\n", resp.StatusCode, string(bodyBytes))
		os.Exit(1)
	}
	fmt.Println("✅ Document uploaded successfully. Status 201 Created.")
}

func getDocumentsWithContent(token, bikeID, providerID string) {
	req, _ := http.NewRequest("GET", baseURL+"/api/v1/provider/vehicle/"+bikeID+"/documents", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Printf("❌ Request error: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		fmt.Printf("❌ GET documents failed status=%d body=%s\n", resp.StatusCode, string(bodyBytes))
		os.Exit(1)
	}

	var parsed map[string]any
	json.Unmarshal(bodyBytes, &parsed)
	docs := parsed["data"].([]any)
	if len(docs) != 1 {
		fmt.Printf("❌ Expected 1 document, got %d\n", len(docs))
		os.Exit(1)
	}

	doc := docs[0].(map[string]any)
	fileURL := doc["file_url"].(string)
	expectedPrefix := fmt.Sprintf("local-private://vehicles/%s/%s/insurance/", providerID, bikeID)
	if !strings.HasPrefix(fileURL, expectedPrefix) {
		fmt.Printf("❌ Document file_url format is incorrect. Expected prefix: %s, got: %s\n", expectedPrefix, fileURL)
		os.Exit(1)
	}
	fmt.Printf("✅ Verified document file_url is in expected format: %s\n", fileURL)
}

func adminReview(token, bikeID, action, reason string) {
	body := map[string]any{
		"action": action,
	}
	if reason != "" {
		body["reason"] = reason
	}
	reqBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("PATCH", baseURL+"/api/v1/admin/vehicle/"+bikeID+"/review", bytes.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Printf("❌ Request error: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		fmt.Printf("❌ Admin review failed status=%d body=%s\n", resp.StatusCode, string(bodyBytes))
		os.Exit(1)
	}
	fmt.Printf("✅ Admin review action '%s' completed successfully.\n", action)
}

func adminReviewExpectConflict(token, bikeID, action, reason string) {
	body := map[string]any{
		"action": action,
	}
	if reason != "" {
		body["reason"] = reason
	}
	reqBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("PATCH", baseURL+"/api/v1/admin/vehicle/"+bikeID+"/review", bytes.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Printf("❌ Request error: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusConflict {
		fmt.Printf("❌ Conflict check failed. Expected 409, got %d. Body: %s\n", resp.StatusCode, string(bodyBytes))
		os.Exit(1)
	}
	fmt.Printf("✅ Conflict check passed. Admin review with redundant action '%s' correctly returned 409 Conflict.\n", action)
}

func verifyEventReceived(mu *sync.Mutex, events *map[string][]string, topic string) {
	mu.Lock()
	msgs := (*events)[topic]
	mu.Unlock()

	if len(msgs) == 0 {
		fmt.Printf("❌ Event for topic '%s' was NOT received in Redis.\n", topic)
		os.Exit(1)
	}
	fmt.Printf("✅ Redis Event Verified: Event published to topic '%s' was successfully received.\n", topic)
}

func verifyBikeDBState(ctx context.Context, conn *pgx.Conn, bikeID, status string, active bool) {
	var dbStatus string
	var dbActive bool
	err := conn.QueryRow(ctx, "SELECT verification_status, is_active FROM bikes WHERE id = $1", bikeID).Scan(&dbStatus, &dbActive)
	if err != nil {
		fmt.Printf("❌ DB Query failed: %v\n", err)
		os.Exit(1)
	}

	if dbStatus != status {
		fmt.Printf("❌ DB state mismatch. Expected status '%s', got '%s'\n", status, dbStatus)
		os.Exit(1)
	}
	if dbActive != active {
		fmt.Printf("❌ DB state mismatch. Expected is_active '%v', got '%v'\n", active, dbActive)
		os.Exit(1)
	}
	fmt.Printf("✅ DB State Verified for bike ID %s: verification_status = '%s', is_active = %v.\n", bikeID, dbStatus, dbActive)
}

func verifyAuditRow(ctx context.Context, conn *pgx.Conn, bikeID, action, notes string) {
	var count int
	var err error
	if notes == "" {
		err = conn.QueryRow(ctx, "SELECT count(*) FROM bike_audit WHERE bike_id = $1 AND action = $2", bikeID, action).Scan(&count)
	} else {
		err = conn.QueryRow(ctx, "SELECT count(*) FROM bike_audit WHERE bike_id = $1 AND action = $2 AND notes = $3", bikeID, action, notes).Scan(&count)
	}
	if err != nil {
		fmt.Printf("❌ DB Query failed for audit: %v\n", err)
		os.Exit(1)
	}

	if count == 0 {
		fmt.Printf("❌ DB Audit row missing for bike ID %s, action '%s', notes '%s'.\n", bikeID, action, notes)
		// Debug print all audit rows for this bike
		rows, qerr := conn.Query(ctx, "SELECT action, from_status, to_status, notes FROM bike_audit WHERE bike_id = $1", bikeID)
		if qerr == nil {
			fmt.Println("Existing audits for this bike:")
			for rows.Next() {
				var act, from, to string
				var nts *string
				_ = rows.Scan(&act, &from, &to, &nts)
				nsVal := "<nil>"
				if nts != nil {
					nsVal = *nts
				}
				fmt.Printf("  - Action: %s | From: %s | To: %s | Notes: %s\n", act, from, to, nsVal)
			}
		}
		os.Exit(1)
	}
	fmt.Printf("✅ DB Audit Row Verified: Found audit record for bike ID %s and action '%s'.\n", bikeID, action)
}
