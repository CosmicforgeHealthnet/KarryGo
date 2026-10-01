// Package custodychain is the cross-service vocabulary for who holds a
// handover credential, who verifies it, and who is holding the goods.
//
// It exists because these three facts must mean exactly the same thing in
// Vendor, Dispatch, Taxi Goods and Hauling, and because getting one of them
// backwards is a security failure rather than a naming inconsistency. A
// Commerce pickup where the VENDOR holds the credential and the PROVIDER
// verifies it would let any provider who shows up collect the goods; a direct
// Taxi pickup with the Commerce direction would let the driver approve their
// own arrival. Both flows already exist in this platform and both already get
// the direction right, but nothing stated the rule in one place where a new
// authority could read it, and nothing failed a build when a route was wired
// the wrong way round.
//
// What this package is NOT: it holds no credential material, generates and
// verifies nothing, and persists nothing. Credential generation and
// verification stay in shared/go/handover and in each authority's own
// storage, where the secret lives. Authority-local custody operations
// (manifests, evidence, recovery, transfer) stay in shared/go/custodyops.
// This package only says what the roles are and which transitions are legal.
package custodychain

import "strings"

// CredentialPurpose names one handover challenge's job.
//
// The purpose is what decides holder and verifier. It is deliberately not
// derived from a route name or an HTTP method: a route can be re-mounted, and
// "the POST on the mission path" is not a security property.
type CredentialPurpose string

const (
	// PurposeCommercePickup is a merchant-originated pickup: the vendor,
	// shop or restaurant is the SENDER and hands goods to a platform
	// provider. The provider carries the credential and the sender checks it
	// before releasing anything.
	PurposeCommercePickup CredentialPurpose = "COMMERCE_PICKUP"

	// PurposeDirectPickup is Taxi's direct (non-Commerce) pickup contract:
	// the passenger or individual sender holds the credential and the Taxi
	// provider verifies it. This is the mirror image of Commerce and must not
	// be replaced by it (CFC-006).
	PurposeDirectPickup CredentialPurpose = "DIRECT_PICKUP"

	// PurposeSenderVerifiedPickup is the third pickup direction this platform
	// actually runs, and it is neither of the two the spec names.
	//
	// Dispatch and Hauling direct (non-Commerce) parcel work protect the
	// CUSTOMER from a driver who is not the assigned one: the provider's app
	// shows the credential and the customer's app verifies it before the
	// parcel is released. Holder and verifier are therefore provider and
	// individual sender — the Commerce roles with a consumer in the vendor's
	// place, not the Taxi direct contract.
	//
	// It is a separate purpose rather than a reuse of COMMERCE_PICKUP because
	// the verifier is a different authentication surface: a bearer-token
	// customer app, not an HMAC-signed Vendor Service. Collapsing them would
	// let a customer session verify a merchant's mission.
	PurposeSenderVerifiedPickup CredentialPurpose = "SENDER_VERIFIED_PICKUP"

	// PurposeDelivery is the final handover to the receiver. The receiver
	// holds the credential; the provider verifies it before custody moves.
	PurposeDelivery CredentialPurpose = "DELIVERY"

	// PurposeReturn is a return leg: goods go back to the original sender,
	// who then verifies the returning provider exactly as at pickup.
	PurposeReturn CredentialPurpose = "RETURN"

	// PurposeProviderTransfer is a provider-to-provider handover during an
	// explicit, supported recovery. Both providers participate; the
	// receiving provider verifies the outgoing provider's credential.
	PurposeProviderTransfer CredentialPurpose = "PROVIDER_TRANSFER"
)

// PartyRole is one side of a handover.
//
// SENDER is split into two roles on purpose. "The sender" is a vendor in one
// flow and a customer in the other, and those are different authentication
// surfaces: a vendor verifies through an HMAC-signed internal call, a
// customer through a bearer token. Collapsing them into one role is how a
// customer-facing route ends up accepting a vendor's mission.
type PartyRole string

const (
	// RoleProvider is the rider, driver or truck driver executing the work.
	RoleProvider PartyRole = "PROVIDER"

	// RoleVendorSender is the vendor, shop or restaurant acting as sender on
	// a Commerce order. It acts through Vendor Service, not through a
	// consumer app.
	RoleVendorSender PartyRole = "VENDOR_SENDER"

	// RoleCustomerSender is an individual sender or passenger on direct
	// work, acting through a consumer app.
	RoleCustomerSender PartyRole = "CUSTOMER_SENDER"

	// RoleReceiver is the party the goods are delivered to.
	RoleReceiver PartyRole = "RECEIVER"

	// RoleExecutionService is the owning execution authority itself, acting
	// on behalf of a provider it has already authenticated.
	RoleExecutionService PartyRole = "EXECUTION_SERVICE"
)

// Descriptor is the authoritative statement of one credential's direction.
//
// Holder is who is given the credential. Verifier is who submits it for
// checking. They are never the same role: a party that both holds and
// verifies is approving itself, which is the whole failure this type exists
// to make impossible to express.
type Descriptor struct {
	Purpose  CredentialPurpose `json:"credentialPurpose"`
	Holder   PartyRole         `json:"holderRole"`
	Verifier PartyRole         `json:"verifierRole"`

	// SingleUse records that a successful verification consumes the
	// credential. Every purpose here is single-use; the field is explicit so
	// a future non-consuming purpose cannot be added silently.
	SingleUse bool `json:"singleUse"`
}

// descriptors is the canonical matrix. It is unexported and read through
// DescriptorFor so no caller can append a sixth direction at runtime.
var descriptors = map[CredentialPurpose]Descriptor{
	PurposeCommercePickup: {
		Purpose: PurposeCommercePickup, Holder: RoleProvider,
		Verifier: RoleVendorSender, SingleUse: true,
	},
	PurposeDirectPickup: {
		Purpose: PurposeDirectPickup, Holder: RoleCustomerSender,
		Verifier: RoleProvider, SingleUse: true,
	},
	PurposeSenderVerifiedPickup: {
		Purpose: PurposeSenderVerifiedPickup, Holder: RoleProvider,
		Verifier: RoleCustomerSender, SingleUse: true,
	},
	PurposeDelivery: {
		Purpose: PurposeDelivery, Holder: RoleReceiver,
		Verifier: RoleProvider, SingleUse: true,
	},
	PurposeReturn: {
		Purpose: PurposeReturn, Holder: RoleProvider,
		Verifier: RoleVendorSender, SingleUse: true,
	},
	PurposeProviderTransfer: {
		Purpose: PurposeProviderTransfer, Holder: RoleProvider,
		Verifier: RoleProvider, SingleUse: true,
	},
}

// DescriptorFor returns the canonical direction for a purpose.
//
// An unknown purpose returns ok=false rather than a zero Descriptor that
// would read as "nobody holds it and nobody verifies it" — a caller that
// ignores ok and uses the result then refuses every handover, which is the
// safe direction, instead of allowing one.
func DescriptorFor(purpose CredentialPurpose) (Descriptor, bool) {
	descriptor, ok := descriptors[purpose]
	return descriptor, ok
}

// ParsePurpose normalizes a caller-supplied purpose.
func ParsePurpose(value string) (CredentialPurpose, bool) {
	purpose := CredentialPurpose(strings.ToUpper(strings.TrimSpace(value)))
	_, ok := descriptors[purpose]
	return purpose, ok
}

// Purposes returns every known purpose. Used by conformance tests that must
// assert over the whole matrix rather than a hand-written list that can drift
// away from it.
func Purposes() []CredentialPurpose {
	return []CredentialPurpose{
		PurposeCommercePickup, PurposeDirectPickup, PurposeSenderVerifiedPickup,
		PurposeDelivery, PurposeReturn, PurposeProviderTransfer,
	}
}

// Valid reports whether the purpose is one this platform knows.
func (p CredentialPurpose) Valid() bool {
	_, ok := descriptors[p]
	return ok
}

// HeldBy reports whether role is the party that carries this credential.
func (d Descriptor) HeldBy(role PartyRole) bool { return d.Holder == role }

// VerifiedBy reports whether role is the party entitled to submit this
// credential for verification.
func (d Descriptor) VerifiedBy(role PartyRole) bool { return d.Verifier == role }

// ProviderTransfer is the one purpose whose holder and verifier are the same
// role, because both parties are providers. It is legal only inside an
// explicit transfer operation, which is why it is called out rather than
// left for a reader to notice.
func (d Descriptor) SameRoleBothSides() bool { return d.Holder == d.Verifier }
