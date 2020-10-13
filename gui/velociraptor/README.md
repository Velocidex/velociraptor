# Velociraptor GUI development

This directory contains the source code for the Velociraptor GUI.

## Setting up a dev platform.

To build the web app you need to first install dependencies using npm:

```
npm i
```

Then start the Velociraptor server. NOTE: By default Velociraptor
listens on SSL and node does not. Therefore the CSRF cookies (which
are marked secure) are not properly set. For testing and development
you can switch CSRF protection off

```
export VELOCIRAPTOR_DISABLE_CSRF=1
velociraptor --config server.config.yaml frontend -v
```

Now you can start the node server:
```
npm run start
```

The development setup starts a node server listening on port 3000, and
proxies the API requests to the Velociraptor server (which should be
listening on port 8889).

The node server will rebuild the application and automatically refresh
it each time the JavaScript file is edited.

NOTE: The first time the code is built, the build will take a
considerable time... dont worry it will get there in the end. Sadly
this is a limitation in webpack being very slow.
