// Package virtualip owns the hub's virtual IP: every connection the hub holds
// terminates on it. It moves the address and its live connections between
// pods during planned operations (socket handover on the same node,
// TCP_REPAIR across nodes). See docs/adr/0005.
package virtualip
