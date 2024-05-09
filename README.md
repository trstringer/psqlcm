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
   new                  New connection
   list, ls             List all available connections
   show                 Show a connection string
   delete, del, remove  Remove a cached connection
   set-current          Set a connection as current
   help, h              Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --help, -h  show help
```

### Add a connection

```
$ psqlcm new
🖥️  Hostname [localhost]: 127.0.0.1
🌐 Port [5432]: 
📝 Database [postgres]: mydb1
🔨 User [postgres]: myuser1
🔑 Password: 
🔒 SSL mode [require]: disable

📕 Connection name [pg1715219721581]: my-connection1
⚡ Test connection [Y/n]: n
Connection saved!
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
