// Package dashboard provides the aggregated dashboard API for the Fleet OS
// frontend. It aggregates data from dispatch, assets, providers, and alerts
// into a single response.
//
// This is the production-grade approach: one optimized endpoint instead of
// the frontend making 4-5 separate calls.
package dashboard