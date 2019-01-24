Python bindings for Velociraptor
================================

Velociraptor uses gRPC to facilitate automation bindings with other
languages. This directory contains the basic framework for controlling
the Velociraptor server using python.

The gRPC API is very simple - it only allows a series of VQL queries
to be issued (or artifacts collected) and stream their results
back. However since Velociraptor can be fully controlled using VQL
this is all that is needed to automate everything about the server.

.. note::

   Server side VQL is all powerful! It allows callers to do everything
   with the server without any limitation. Callers can collect
   artifacts on arbitrary clients in your deployment. You must protect
   access to the API as explained below.

Security
--------

By default the API is bound to the loopback address 127.0.0.1. You can
change this by setting `API.bind_address` to 0.0.0.0 in the config
file. Regardless, the API is protected with TLS and clients must be
authenticated using certificates.

Velociraptor uses mutual authentication - the client verifies that the
server's certificate is signed by the CA and the server verifies that
the client presents certificate signed by the CA. At the core of this
scheme is the security of the Velociraptor CA (i.e. the CA.private_key
field in the server.config.yaml file).

When you first create the Velociraptor configuration (using
`velociraptor config generate`) the CA private_key is also included in
the config file. For extra secure deployments you should keep that
entire config file offline, and reduct that field from the running
server. This way you will only be able to sign offline, even if the
server were compromised.

Before you may connect to the API you will need a "api_client"
configuration file. This file is a simple YAML file which contains the
relevant keys and certificates to authenticate to the API
endpoint. You can prepare one from a server config containing the ca:

.. code-block::

   $ velociraptor --config server.config.yaml \
       config api_client --name fred > freds_api_client.yaml

This command generates the relevant certificates to allow the API
client to connect. The name "Fred" is the common name of the
certificate (this name will appear in server Auditing logs). Typically
the API clients are automated processes and you can distinguish
between different automated processes using the common name.

You may now use this yaml file to connect to the API endpoint - see
the client_example.py for an example of a program which runs a query.


Example queries
---------------

This API is mainly meant for post processing files on the server in a
more flexible way. You can set up server side event queries and
process them using python code.

For example consider this query:

.. code::

   SELECT * FROM watch_monitoring(artifact='System.Flow.Completion')


This will emit a single row which contains the flow information for
each flow that is completed (from any client). You can then test the
flow object to act on specific artifacts collected for example. The
query will continue monitoring new flows as they occur.

The following code will launch an artifact collection against a client:

.. code::

   SELECT collect(client_id="C.12345", artifacts="Windows.Network.Netstat")
   FROM scope()
