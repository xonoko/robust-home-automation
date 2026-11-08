// Atlas configuration for versioned migrations (docs/adr/0004).
// Migrations are generated from the ent schema and must be expand-then-contract:
// two hub versions run side by side during a rollout.

env "local" {
  src = "ent://internal/store/ent/schema"
  // Disposable Postgres used by Atlas to compute diffs. Requires Docker.
  dev = "docker://postgres/18/dev?search_path=public"
  migration {
    dir = "file://internal/store/migrations"
  }
  format {
    migrate {
      diff = "{{ sql . \"  \" }}"
    }
  }
}
