This is a distributed flat file storage highly inspired by the course video by [Anthony](https://youtu.be/IoY6bE--A54?si=ocsBTyBIF8iDi-uP).

Run the following command to run it locally.

```sh
make run
```

By default it starts 3 file servers in the port 3000, 4000 and 6000. Corresponding administrative ports are 3001, 4001 and 6001.

The former(file sockets) are used by the fileservers to communicate with each other and share files while administrative ports can be used
by the clients to connect and read/write files.

The server with port 3000 starts with known peers 4000 and 6000 and shares any file stored in it.

A seed file is also created on server 3000 to demonstrate "read file" feature.

After all the servers are up, you can connect and interact with these servers using netcat tool.

To list all availble files:

```sh
❯ echo -n "ls\0" | nc localhost 3001
```

`\0` is required to mark the end of a command.

Similarly to read that file

```sh
❯ echo -n "cat seedfilename\0" | nc localhost 3001
this is a seed data file to be written
```

And store some text file use the following:

```sh
❯ echo -n "write myfilename\0This is a new file to be created in all the distributed servers" | nc localhost 3001
successfully stored myfilename
```
