package matching

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

func testRedis(t *testing.T) *redis.Client {
	t.Helper()
	addr := os.Getenv("TAXI_TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("TAXI_TEST_REDIS_ADDR is required for booking matcher-lease tests")
	}
	client := redis.NewClient(&redis.Options{Addr: addr})
	if err := client.Ping(context.Background()).Err(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func TestAcquire_SecondInstanceCannotClaimAHeldLease(t *testing.T) {
	client := testRedis(t)
	lease := NewMatcherLease(client)
	bookingID := uuid.NewString()
	t.Cleanup(func() { _ = client.Del(context.Background(), leaseKey(bookingID)).Err() })

	ok, err := lease.Acquire(context.Background(), bookingID, "instance-a", time.Minute)
	if err != nil || !ok {
		t.Fatalf("first Acquire: ok=%v err=%v, want ok=true", ok, err)
	}

	ok, err = lease.Acquire(context.Background(), bookingID, "instance-b", time.Minute)
	if err != nil {
		t.Fatalf("second Acquire: %v", err)
	}
	if ok {
		t.Error("a second instance claimed a lease already held by another -- this is the exact defect class addendum §35.1 requires closed")
	}
}

func TestAcquire_SucceedsAgainAfterRelease(t *testing.T) {
	client := testRedis(t)
	lease := NewMatcherLease(client)
	bookingID := uuid.NewString()
	t.Cleanup(func() { _ = client.Del(context.Background(), leaseKey(bookingID)).Err() })

	if ok, err := lease.Acquire(context.Background(), bookingID, "instance-a", time.Minute); err != nil || !ok {
		t.Fatalf("first Acquire: ok=%v err=%v", ok, err)
	}
	if err := lease.Release(context.Background(), bookingID, "instance-a"); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if ok, err := lease.Acquire(context.Background(), bookingID, "instance-b", time.Minute); err != nil || !ok {
		t.Fatalf("Acquire after release: ok=%v err=%v, want ok=true", ok, err)
	}
}

func TestRelease_WrongOwnerCannotReleaseSomeoneElsesLease(t *testing.T) {
	client := testRedis(t)
	lease := NewMatcherLease(client)
	bookingID := uuid.NewString()
	t.Cleanup(func() { _ = client.Del(context.Background(), leaseKey(bookingID)).Err() })

	if ok, err := lease.Acquire(context.Background(), bookingID, "instance-a", time.Minute); err != nil || !ok {
		t.Fatalf("Acquire: ok=%v err=%v", ok, err)
	}
	// instance-b never held this lease and must not be able to remove it.
	if err := lease.Release(context.Background(), bookingID, "instance-b"); err != nil {
		t.Fatalf("Release (wrong owner, should no-op not error): %v", err)
	}
	// instance-a's lease must still stand -- proven by instance-c failing to acquire.
	ok, err := lease.Acquire(context.Background(), bookingID, "instance-c", time.Minute)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if ok {
		t.Error("the lease was removed by a non-owner's Release call")
	}
}

func TestRenew_WrongOwnerCannotExtendSomeoneElsesLease(t *testing.T) {
	client := testRedis(t)
	lease := NewMatcherLease(client)
	bookingID := uuid.NewString()
	t.Cleanup(func() { _ = client.Del(context.Background(), leaseKey(bookingID)).Err() })

	if ok, err := lease.Acquire(context.Background(), bookingID, "instance-a", time.Minute); err != nil || !ok {
		t.Fatalf("Acquire: ok=%v err=%v", ok, err)
	}
	renewed, err := lease.Renew(context.Background(), bookingID, "instance-b", time.Minute)
	if err != nil {
		t.Fatalf("Renew: %v", err)
	}
	if renewed {
		t.Error("a non-owner's Renew call reported success")
	}
}

func TestRenew_RealOwnerExtendsSuccessfully(t *testing.T) {
	client := testRedis(t)
	lease := NewMatcherLease(client)
	bookingID := uuid.NewString()
	t.Cleanup(func() { _ = client.Del(context.Background(), leaseKey(bookingID)).Err() })

	if ok, err := lease.Acquire(context.Background(), bookingID, "instance-a", 2*time.Second); err != nil || !ok {
		t.Fatalf("Acquire: ok=%v err=%v", ok, err)
	}
	renewed, err := lease.Renew(context.Background(), bookingID, "instance-a", time.Minute)
	if err != nil {
		t.Fatalf("Renew: %v", err)
	}
	if !renewed {
		t.Error("the real owner's Renew call reported failure")
	}
	ttl, err := client.TTL(context.Background(), leaseKey(bookingID)).Result()
	if err != nil {
		t.Fatalf("TTL: %v", err)
	}
	if ttl < 30*time.Second {
		t.Errorf("TTL after renew = %v, want close to 1 minute (renew should have extended past the original 2s)", ttl)
	}
}
