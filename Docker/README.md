# Velociraptor docker container

This directory builds a docker container for launching Velociraptor.

This container is designed for a couple of use cases:

1. No prior Velociraptor deployment. Spin up Velociraptor easily with
   default everything.

   This use case creates a new configuration file with:
   * Self signed certificates
   * GUI port by default is listening on 8889, Frontend port listening on 8000
   * Generate a new configuration file stored in the /etc/ directory
   * Mounts the datastore in the /datastore/ - this allows the deployment data to persist.
   * The deployment will trigger a build for client assets like MSI, Deb and RPM packages
   * An initial user is created with admin permissions. Default
     password is `password` or take from the .env file, but you should
     change it from the GUI after the server is up.

2. An existing Velociraptor deployment with a pre-configured configuration file.

   In this case, simply copy your configuration file to the `etc/`
   directory and adjust the .env file to match the forwarded ports.


In both cases the `datastore` directory is used as permanent storage
and remains after the container is terminated. It is safe to delete
the datastore and start fresh at any time - clients will just
re-connect and re-enrol.


## Quick start

The CI pipeline uploads the container to the GitHub container
registry, so all you need to do is copy the `compose.yaml` from this
directory and simply run:

```
docker-compose up
```

The latest image will be fetched from the registry, a default
configuration will be generated and the server will be started.

You can connect to the GUI on `https://localhost:8889/` with default
password of `password`. You can tweak the `.env` file to update this
default password.

The datastore files will be stored in the `datastore` directory and
the generated config file will be stored in `etc`. Make sure to back
up the generated config file to ensure existing clients can still talk
to this server.
