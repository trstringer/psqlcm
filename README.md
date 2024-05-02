# psqlcm (psql connection manager)

Manage your PostgreSQL connections locally.

## Build

```
$ make build
```

Copy `./bin/psqlcm` to a path dir.

## Usage

Add a new connection: `$ psqlcm login`

List all connections: `$ psqlcm ls`

Show a connection string: `$ psqlcm show <connection_name>`

Delete a connection: `$ psqlcm delete <connection_name>`
