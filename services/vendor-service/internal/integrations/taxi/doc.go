// Package taxi documents Vendor's integration boundary with the distinct Taxi
// Goods authority in driver-taxi-service. The executable clients are the
// shared commercequote and commercemission clients wired in cmd, so this
// package deliberately owns no duplicate wire types or pricing logic.
//
// Vendor selects only the TAXI authority and CAR requirement from an
// authoritative quote. It never selects a provider, mutates Taxi state,
// generates handover credentials, stores evidence bytes, or charges money.
// Both services gate this integration with TAXI_GOODS_DELIVERY_ENABLED, false
// by default. Passenger /customer/bookings and ride_purpose=parcel are not used.
package taxi
