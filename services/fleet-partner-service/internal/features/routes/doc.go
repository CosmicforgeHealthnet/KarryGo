// Package routes owns Section 16 route planning: route plans, ordered stops,
// optimization through a provider-neutral optimizer, approval, publication
// into Section 15 private assignments, replanning and route progress.
// Publication creates the stop assignments in the same transaction as the
// route change; canonical trips remain owned by Dispatch and Hauling.
//
// Layout: models (route domain and stop rules), repositories (SQL), usecases
// (workflows, workers and the ports to dispatch, eligibility and private
// assignments), clients (the optimizer transport) and http (/routes...).
package routes
