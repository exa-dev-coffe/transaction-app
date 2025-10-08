# Go Fiber with golang-migrate Example

Install the golang migrate CLI tool if you haven't already:

```bash 
go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
```

# Create migrations

Create a new migration file using the migrate CLI tool:

```bash 
migrate create -ext sql -dir db/migrations -seq create_users_table
```

This will create two files in the `db/migrations` directory: `xxxxxx_create_users_table.up.sql` and
`xxxxxx_create_users_table.down.sql`.

# Write migration SQL

Edit the generated migration files to define the schema changes. For example, in `xxxxxx_create_users_table.up.sql`:

```sql
CREATE TABLE users
(
    id    SERIAL PRIMARY KEY,
    name  VARCHAR(100) NOT NULL,
    email VARCHAR(100) NOT NULL UNIQUE
);
```

And in `xxxxxx_create_users_table.down.sql`:

```sql
DROP TABLE users;
``` 

# Run migrations

Run the migrations using the migrate CLI tool:

```bash 
migrate -path db/migrations -database "postgres://username:password@localhost:5432/dbname?sslmode=disable" up
```

Replace `username`, `password`, and `dbname` with your PostgreSQL credentials and database name.
This will apply all up migrations that have not yet been applied to the database.
To rollback the last migration, you can use:

```bash 
migrate -path db/migrations -database "postgres://username:password@localhost:5432/dbname?sslmode=disable" down
```

if error dirty database, you can use:

```bash 
migrate -path db/migrations -database "postgres://username:password@localhost:5432/dbname?sslmode=disable" force <version>
```

# Or Usage in Go code for migrations create/up/down/force

this list of functions is available:

```go
go run cmd/migrate/main.go create create_users_table
go run cmd/migrate/main.go up
go run cmd/migrate/main.go down
go run cmd/migrate/main.go force <version>
```




