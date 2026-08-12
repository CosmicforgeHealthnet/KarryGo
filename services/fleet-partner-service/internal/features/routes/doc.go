// Package routes documents the Section 16 route-planning boundary. Runtime
// orchestration lives in the dispatch feature so publication, assignments and
// control-tower updates share one transaction boundary; canonical trips remain
// owned by Dispatch and Hauling.
package routes
