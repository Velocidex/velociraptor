# Velociraptor GUI development

This directory contains the source code for the Velociraptor GUI.

## Setting up a dev platform.

To build the web app you need to first install dependencies using npm:

```
npm install
```

Then start the Velociraptor server. NOTE: By default Velociraptor
listens on SSL and node does not. Therefore the CSRF cookies (which
are marked secure) are not properly set. For testing and development
you can switch CSRF protection off.

```
export VELOCIRAPTOR_DISABLE_CSRF=1
velociraptor --config server.config.yaml frontend -v
```

Now you can start the node server:
```
npm run start
```

The development setup starts a node server listening on port 3000, and
proxies the API requests to the Velociraptor server, which by default is
expected to be listening on port 8889.

If you want the API requests to be forwarded to a remote server or on a
different port then edit the value of `server.proxy` in vite.config.js.

The node dev server will rebuild the application and automatically refresh
it each time a source file is edited.
