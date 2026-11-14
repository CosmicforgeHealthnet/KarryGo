// Package warehouse owns tenant-scoped warehouses, customer stock,
// fulfilment, dock, yard, terminal and return operations. Maintenance parts
// remain owned by Section 18 and canonical trips remain external.
//
// Layout: models (domain types), repositories (PostgreSQL), usecases
// (workflows, behind a Repository interface) and http (routes and DTOs,
// behind a UseCases interface).
package warehouse
