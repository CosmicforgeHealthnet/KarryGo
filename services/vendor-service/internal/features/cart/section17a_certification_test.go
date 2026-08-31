package cart

import (
	"context"
	"errors"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/vendor-service/internal/events"
	"cosmicforge/logistics/services/vendor-service/internal/platform/idempotency"
	sharedauth "cosmicforge/logistics/shared/go/auth"
)

func section17ACertRef(prefix string) string {
	return prefix + strings.ReplaceAll(uuid.NewString(), "-", "")
}

func section17ACertPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	dsn := strings.TrimSpace(os.Getenv("VENDOR_SERVICE_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("VENDOR_SERVICE_TEST_DATABASE_URL not set; skipping Section 17A PostgreSQL certification")
	}

	parsed, err := url.Parse(dsn)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") {
		t.Fatalf("refusing invalid Section 17A test database URL")
	}

	databaseName := strings.TrimPrefix(parsed.Path, "/")
	host := strings.ToLower(parsed.Hostname())
	localHost := host == "localhost" || host == "127.0.0.1" || host == "::1" || host == "postgres" || host == "db" || strings.HasSuffix(host, "-db")
	if !localHost && os.Getenv("VENDOR_SERVICE_TEST_ALLOW_REMOTE_ADMIN") != "1" {
		t.Fatalf("refusing non-local Section 17A test database host %q", host)
	}
	switch strings.ToLower(databaseName) {
	case "", "postgres", "template0", "template1", "vendor_service":
		t.Fatalf("refusing non-test Section 17A database %q", databaseName)
	}
	if !strings.Contains(strings.ToLower(databaseName), "test") && !strings.HasPrefix(strings.ToLower(databaseName), "vendor_iso_") && !strings.HasPrefix(strings.ToLower(databaseName), "vendor_s8") {
		t.Fatalf("refusing unrecognised Section 17A test database %q", databaseName)
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("Section 17A database connection: %v", err)
	}

	t.Cleanup(pool.Close)

	if err = pool.Ping(ctx); err != nil {
		t.Fatalf("Section 17A database ping: %v", err)
	}

	var migrationApplied bool

	if err = pool.QueryRow(
		ctx,
		`SELECT EXISTS(
            SELECT 1
            FROM vendor_service_schema_migrations
            WHERE version='000075_vendor_customer_cart_foundation'
        )`,
	).Scan(&migrationApplied); err != nil {
		t.Fatalf("Section 17A migration-state read: %v", err)
	}

	if !migrationApplied {
		t.Fatal("Section 17A migration 000075 is not applied to the certification database")
	}

	for _, table := range []string{
		"commerce_carts",
		"commerce_cart_addresses",
		"commerce_cart_receivers",
		"commerce_cart_items",
		"commerce_cart_item_options",
		"commerce_cart_history",
		"vendor_idempotency_keys",
		"vendor_outbox",
	} {
		var exists bool

		if err = pool.QueryRow(
			ctx,
			`SELECT to_regclass('public.'||$1) IS NOT NULL`,
			table,
		).Scan(&exists); err != nil {
			t.Fatalf("table %s existence: %v", table, err)
		}

		if !exists {
			t.Fatalf("required Section 17A table %s is missing", table)
		}
	}

	return pool
}

func section17ASeedCart(
	t *testing.T,
	pool *pgxpool.Pool,
	customerID string,
	domain Domain,
) string {
	t.Helper()

	id := section17ACertRef("cart_cert_")

	_, err := pool.Exec(
		context.Background(),
		`
        INSERT INTO commerce_carts(
            id,
            customer_id,
            domain,
            status,
            version,
            expires_at
        )
        VALUES($1,$2,$3,'ACTIVE',1,now()+interval '1 hour')
        `,
		id,
		customerID,
		domain,
	)
	if err != nil {
		t.Fatalf("seed Section 17A cart: %v", err)
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(
			context.Background(),
			`DELETE FROM vendor_outbox WHERE aggregate_id=$1`,
			id,
		)

		_, _ = pool.Exec(
			context.Background(),
			`DELETE FROM commerce_carts WHERE id=$1`,
			id,
		)
	})

	return id
}

func TestSection17ACertification_CustomerIsolation(t *testing.T) {
	pool := section17ACertPool(t)

	customerA := section17ACertRef("customer_a_")
	customerB := section17ACertRef("customer_b_")

	cartID := section17ASeedCart(
		t,
		pool,
		customerA,
		DomainShop,
	)

	service := NewService(
		NewStore(pool),
		nil,
		nil,
		nil,
	)

	if _, err := service.GetCart(
		context.Background(),
		customerA,
		cartID,
	); err != nil {
		t.Fatalf("owner cannot read cart: %v", err)
	}

	_, err := service.GetCart(
		context.Background(),
		customerB,
		cartID,
	)

	if !errors.Is(err, ErrCartNotFound) {
		t.Fatalf(
			"cross-customer read err=%v want ErrCartNotFound",
			err,
		)
	}
}

func TestSection17ACertification_VendorStaffClaimsFailClosed(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	c.Set(
		sharedauth.ClaimsContextKey,
		sharedauth.Claims{
			Subject:   section17ACertRef("staff_"),
			Role:      "owner",
			Service:   "vendor",
			SessionID: section17ACertRef("session_"),
			Type:      sharedauth.TokenTypeAccess,
		},
	)

	if _, ok := customerCartSubject(c); ok {
		t.Fatal("Vendor staff claims were accepted as a customer principal")
	}

	if recorder.Code < 400 {
		t.Fatalf(
			"Vendor staff claims did not fail closed; status=%d",
			recorder.Code,
		)
	}
}

func TestSection17ACertification_CustomerScopedIdempotency(t *testing.T) {
	pool := section17ACertPool(t)
	ctx := context.Background()

	customerA := section17ACertRef("idem_a_")
	customerB := section17ACertRef("idem_b_")

	endpoint := "POST /api/v1/commerce/carts/:cartId/abandon"
	key := section17ACertRef("key_")

	store := idempotency.NewStore(
		pool,
		time.Hour,
	)

	body := []byte(`{}`)
	fingerprint := idempotency.Fingerprint(
		"POST",
		"/api/v1/commerce/carts/cart/abandon",
		body,
	)

	record, err := store.Begin(
		ctx,
		customerA,
		endpoint,
		key,
		fingerprint,
	)
	if err != nil {
		t.Fatal(err)
	}

	if record != nil {
		t.Fatal("first idempotency claim unexpectedly replayed")
	}

	response := []byte(`{"success":true}`)

	if err = store.Complete(
		ctx,
		customerA,
		endpoint,
		key,
		200,
		response,
	); err != nil {
		t.Fatal(err)
	}

	replay, err := store.Begin(
		ctx,
		customerA,
		endpoint,
		key,
		fingerprint,
	)
	if err != nil {
		t.Fatal(err)
	}

	if replay == nil ||
		replay.ResponseStatus != 200 ||
		string(replay.ResponseBody) != string(response) {
		t.Fatalf("invalid replay record: %+v", replay)
	}

	different := idempotency.Fingerprint(
		"POST",
		"/api/v1/commerce/carts/cart/abandon",
		[]byte(`{"different":true}`),
	)

	if _, err = store.Begin(
		ctx,
		customerA,
		endpoint,
		key,
		different,
	); !errors.Is(err, idempotency.ErrFingerprintMismatch) {
		t.Fatalf(
			"different fingerprint err=%v want ErrFingerprintMismatch",
			err,
		)
	}

	otherCustomer, err := store.Begin(
		ctx,
		customerB,
		endpoint,
		key,
		fingerprint,
	)
	if err != nil {
		t.Fatalf(
			"same key in another customer scope failed: %v",
			err,
		)
	}

	if otherCustomer != nil {
		t.Fatal("another customer unexpectedly replayed Customer A's command")
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(
			context.Background(),
			`
            DELETE FROM vendor_idempotency_keys
            WHERE scope_id=$1 OR scope_id=$2
            `,
			customerA,
			customerB,
		)
	})
}

func TestSection17ACertification_OptimisticConcurrency(t *testing.T) {
	pool := section17ACertPool(t)

	customerID := section17ACertRef("occ_customer_")

	cartID := section17ASeedCart(
		t,
		pool,
		customerID,
		DomainShop,
	)

	service := NewService(
		NewStore(pool),
		nil,
		nil,
		nil,
	)

	start := make(chan struct{})
	results := make(chan error, 2)

	var wg sync.WaitGroup
	wg.Add(2)

	run := func() {
		defer wg.Done()

		<-start

		_, err := service.ClearCart(
			context.Background(),
			customerID,
			cartID,
			1,
		)

		results <- err
	}

	go run()
	go run()

	close(start)

	wg.Wait()
	close(results)

	successes := 0
	conflicts := 0

	for err := range results {
		switch {
		case err == nil:
			successes++

		case errors.Is(err, ErrVersionConflict):
			conflicts++

		default:
			t.Fatalf("unexpected concurrent result: %v", err)
		}
	}

	if successes != 1 || conflicts != 1 {
		t.Fatalf(
			"successes=%d conflicts=%d want exactly 1/1",
			successes,
			conflicts,
		)
	}

	var version int64

	if err := pool.QueryRow(
		context.Background(),
		`SELECT version FROM commerce_carts WHERE id=$1`,
		cartID,
	).Scan(&version); err != nil {
		t.Fatal(err)
	}

	if version != 2 {
		t.Fatalf("cart version=%d want=2", version)
	}
}

func TestSection17ACertification_OutboxIsTransactional(t *testing.T) {
	pool := section17ACertPool(t)

	customerID := section17ACertRef("event_customer_")

	publisher := events.NewDomainPublisher(
		events.NewOutboxRepository(pool),
	)

	service := NewService(
		NewStore(pool),
		nil,
		nil,
		nil,
		publisher,
	)

	created, err := service.CreateOrGetActiveCart(
		context.Background(),
		customerID,
		DomainShop,
	)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(
			context.Background(),
			`DELETE FROM vendor_outbox WHERE aggregate_id=$1`,
			created.ID,
		)

		_, _ = pool.Exec(
			context.Background(),
			`DELETE FROM commerce_carts WHERE id=$1`,
			created.ID,
		)
	})

	var eventType string
	var aggregateVersion int64
	var vendorID string
	var payload string

	if err = pool.QueryRow(
		context.Background(),
		`
        SELECT
            event_type,
            aggregate_version,
            COALESCE(vendor_id,''),
            payload::text
        FROM vendor_outbox
        WHERE aggregate_type='commerce_cart'
          AND aggregate_id=$1
        ORDER BY created_at
        LIMIT 1
        `,
		created.ID,
	).Scan(
		&eventType,
		&aggregateVersion,
		&vendorID,
		&payload,
	); err != nil {
		t.Fatalf("read cart outbox event: %v", err)
	}

	if eventType != "commerce.cart.created" {
		t.Fatalf("event_type=%q", eventType)
	}

	if aggregateVersion != 1 {
		t.Fatalf(
			"aggregate_version=%d want=1",
			aggregateVersion,
		)
	}

	if vendorID != "" {
		t.Fatalf(
			"multi-vendor customer cart event must be platform scoped; vendor_id=%q",
			vendorID,
		)
	}

	if !strings.Contains(payload, customerID) {
		t.Fatal("cart event lacks opaque customer reference")
	}

	for _, forbidden := range []string{
		"receiver_phone",
		"receiver_name",
		"phone_number",
	} {
		if strings.Contains(
			strings.ToLower(payload),
			forbidden,
		) {
			t.Fatalf(
				"receiver PII-shaped field leaked into event: %s",
				payload,
			)
		}
	}
}

type section17AFailingPublisher struct{}

func (section17AFailingPublisher) Publish(
	context.Context,
	pgx.Tx,
	string,
	string,
	string,
	int64,
	string,
	string,
	string,
	any,
) error {
	return errors.New("forced Section 17A publisher failure")
}

func TestSection17ACertification_PublisherFailureRollsBackCart(t *testing.T) {
	pool := section17ACertPool(t)

	customerID := section17ACertRef("rollback_customer_")

	service := NewService(
		NewStore(pool),
		nil,
		nil,
		nil,
		section17AFailingPublisher{},
	)

	_, err := service.CreateOrGetActiveCart(
		context.Background(),
		customerID,
		DomainShop,
	)

	if err == nil {
		t.Fatal("expected publisher failure")
	}

	var cartCount int

	if err = pool.QueryRow(
		context.Background(),
		`
        SELECT count(*)
        FROM commerce_carts
        WHERE customer_id=$1
        `,
		customerID,
	).Scan(&cartCount); err != nil {
		t.Fatal(err)
	}

	if cartCount != 0 {
		t.Fatalf(
			"publisher failure committed %d cart rows",
			cartCount,
		)
	}
}
