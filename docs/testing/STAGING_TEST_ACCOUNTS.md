# Staging test accounts

Ready-made accounts on the staging server (`https://api-gateway.staging.karryam.com`)
so anyone can sign in to the customer, truck and taxi apps and find real-looking
data straight away.

## Signing in

Sign in with the phone number below. These numbers do not receive SMS: each one
accepts the **staging test code**, a fixed 6-digit code. Ask the team for it;
it is deliberately not written in this public repository.

Only these reserved numbers (the `+234 800 00…` block, never personal mobiles)
accept the test code. Any other number still needs a real code, and the switch
cannot be turned on in production.

## Accounts

### Customer app

| Phone | Name | What's there |
|---|---|---|
| 0800 000 1001 | Tolu Tester | ₦50,000 in the wallet, past truck jobs and taxi rides, notifications |
| 0800 000 1002 | Bimpe Tester | ₦25,000 in the wallet, no history yet |

### Truck app

| Phone | Name | What's there |
|---|---|---|
| 0800 000 2001 | Musa Haulage | Approved. 2 trucks, 6 completed and 1 cancelled job, customer reviews, ₦107,695 in the wallet, notifications |
| 0800 000 2002 | Kemi Carriers | Approved. 1 truck, 2 completed jobs, ₦14,705 in the wallet |
| 0800 000 2003 | Femi Newdriver | Awaiting approval: documents in review, truck not yet active, so he cannot go online |

### Taxi app

| Phone | Name | What's there |
|---|---|---|
| 0800 000 3001 | Ngozi Taxi | Approved, comfort car (Toyota Camry). 6 completed and 1 cancelled ride, two-way reviews, ₦31,195 in the wallet, notifications |
| 0800 000 3002 | Segun Rides | Approved, economy car (Toyota Corolla). 2 completed rides, ₦4,462.50 in the wallet |
| 0800 000 3003 | Halima Newdriver | Awaiting approval: identity, documents and car pending, so she cannot go online |

## Trying a live job

1. Sign in to the truck or taxi app as an approved driver and go online.
2. Sign in to the customer app as Tolu Tester and book a truck or a ride in Lagos.
   For a truck job, choose a receiver who has a KarryGo account (for a quick
   test, Tolu herself): pickup refuses receivers without one.
3. Accept the request on the driver app and follow it through pickup and delivery.

## Notes

- Wallet withdrawals need a real bank account; Paystack test mode does not
  recognise made-up account numbers.
- The data is loaded by `deploy/platform/bin/seed-staging.sh` from
  `deploy/platform/seeds/staging/`. It is safe to run again: nothing that
  already exists is changed, and it refuses to run anywhere but staging.
- Turning the test numbers on: set `CUSTOMER_TEST_LOGIN_PHONES`,
  `HAULING_TEST_LOGIN_PHONES`, `TAXI_TEST_LOGIN_PHONES` and the matching
  `*_TEST_LOGIN_CODE` in each service's settings on the server, then
  `bin/apply-env.sh <service>`.
