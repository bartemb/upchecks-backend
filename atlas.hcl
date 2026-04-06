env "local" {
  src = "file://internal/sql/schema/schema.sql"
  url = "postgres://postgres:postgres@localhost:5433/upchecks?sslmode=disable"

  dev = "docker://postgres/17/dev"
}
