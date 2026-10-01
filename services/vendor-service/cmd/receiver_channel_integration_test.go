package main

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/vendor-service/internal/database"
	"cosmicforge/logistics/services/vendor-service/internal/features/orders"
)

func TestFrozenReceiverAccountBinding(t *testing.T) {
	dsn := os.Getenv("VENDOR_SERVICE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("VENDOR_SERVICE_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := database.ApplyMigrations(ctx, pool, "../migrations"); err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	cartID, senderID, receiverID := "receiver16-"+uuid.NewString(), uuid.NewString(), uuid.NewString()
	if _, err = tx.Exec(ctx, `INSERT INTO commerce_carts(id,customer_id,domain,expires_at) VALUES($1,$2,'SHOP',$3)`, cartID, senderID, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	phoneOnly := orders.QuoteSource{CartID: cartID, CustomerID: senderID, CartVersion: 1}
	if _, err = tx.Exec(ctx, `INSERT INTO commerce_cart_receivers(cart_id,receiver_name,receiver_phone) VALUES($1,'Third party','+390001234567')`, cartID); err != nil {
		t.Fatal(err)
	}
	if err = loadSection17EReceiver(ctx, tx, &phoneOnly); err != nil {
		t.Fatal(err)
	}
	if !phoneOnly.HasReceiver || phoneOnly.Receiver.ReceiverCustomerID != "" {
		t.Fatalf("phone-only receiver was given account binding: %+v", phoneOnly.Receiver)
	}
	if _, err = tx.Exec(ctx, `UPDATE commerce_cart_receivers SET receiver_customer_id=$2 WHERE cart_id=$1`, cartID, receiverID); err != nil {
		t.Fatal(err)
	}
	accountBound := orders.QuoteSource{CartID: cartID, CustomerID: senderID, CartVersion: 1}
	if err = loadSection17EReceiver(ctx, tx, &accountBound); err != nil {
		t.Fatal(err)
	}
	if !accountBound.HasReceiver || accountBound.Receiver.ReceiverCustomerID != receiverID || accountBound.Receiver.ReceiverCustomerID == senderID {
		t.Fatalf("receiver account binding lost: %+v", accountBound.Receiver)
	}
}
