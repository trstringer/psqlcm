# psqlcm (psql connection manager)

Manage your PostgreSQL connections locally.

## Build

```
$ make build
```

Copy `./bin/psqlcm` to a path dir.

Because `psqlcm` encrypts the password before storing it, you must set `PSQLCM_KEY` prior to running `login` and `show`.

## Usage

```
NAME:
   psqlcm - psql connection manager

USAGE:
   psqlcm [global options] command [command options] 

COMMANDS:
   login     Login and save credentials
   list, ls  List all available connections
   show      Show a connection string
   delete    Remove a cached connection
   help, h   Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --help, -h  show help
```

### Add a connection

```
$ psqlcm login
🖥️ Hostname [localhost]: host1
🌐 Port [5432]: 
📝 Database [postgres]: db1
🔨 User [postgres]: user1
🔒 Password: mysecretpassword   
📕 Connection name [pg1714646843370]: new-connection1
```

*Note: The password is encrypted and cached locally. The plaintext password is never stored.*

### List all connections

```
$ psqlcm ls
new-connection1
```

### Show a connection string

```
$ psqlcm show new-connection1
postgresql://user1:mysecretpassword@host1:5432/db1
```

### Delete a connection

```
$ psqlcm delete new-connection1
Connection "new-connection1" deleted
```
