# Velociraptor - Endpoint visibility and collection tool.

Velociraptor is a tool for collecting host based state information
using Velocidex Query Language (VQL) queries.

Velociraptor is loosely based on Google's GRR technologies but is a
re-implementation and redesign focusing on ease of use, scalability
and flexibility.

To learn more about Velociraptor, read about it on our blog:

   https://velociraptor-blog.velocidex.com

## Quick start

1. Download the binary from the release page.
2. You need to generate a server config file. This will generate new key material:  

    ```bash
  $ velociraptor config generate > /etc/velociraptor.config.yaml
```  

3. Edit the config file and update any settings.  
4. To be able to log into the GUI you will need to make a user account with password.  

    ```bash
     $ velociraptor --config /path/to/my/server.config.yaml user add my_user_name
    ```
    
5. Start the server:
    ```bash   
     $ velociraptor --config /path/to/my/server.config.yaml frontend
    ```
    
6. Point a browser at the GUI port that you set in the config
   file. You should be able to log in with the password set earlier.
7. Generate a client config (this is just the client part of the
   server config you made before - it contains no secrets and can be
   installed on clients.):

    ```bash   
     $ velociraptor --config /path/to/my/server.config.yaml config client > client.conf.yaml
    ```
    
8. Launch the client on any system with this client config file.

    ```bash   
    $ velociraptor --config client.conf.yaml client
    ```
   
9. You should be able to search for the client in the GUI, browse VFS,
   download files etc.

## Building from source.

To build from source, make sure you have a recent Golang installed then just:

1. Get the code:

```bash
   $ go get www.velocidex.com/golang/velociraptor
   $ cd $GO_PATH/go/src/www.velocidex.com/golang/velociraptor/
   $ dep ensure

   # This will build the GUI elements:
   $ cd gui/static/
   $ npm install
   $ gulp compile
   $ cd -

   # This builds a release (i.e. it will embed the GUI files in the
   # binary). If you dont care about the GUI a simple "make" will
   # build a bare binary.
   $ make release
   $ make windows
```

## Getting help

Questions and feedback are welcome at velociraptor-discuss@googlegroups.com

File issues on https://gitlab.com/velocidex/velociraptor
