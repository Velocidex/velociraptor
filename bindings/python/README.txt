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
   access to the API. By default the API is served from a unix domain
   socket and not over TCP.


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
