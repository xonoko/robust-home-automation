// Package maintenance orchestrates planned operations (node drain, reboot,
// upgrades) so that nothing is lost: it moves the virtual IP, integration leases,
// NATS leaders and the Postgres primary off a node before it goes down.
// See docs/adr/0005.
package maintenance
