# Experimental LSP server

This is an experimental LSP server to allow editing VQL in editors
that support LSP (e.g. emacs, VS Code, vim etc).

## How to use it?

The LSP server consists of two components:

1. The LSP server is a binary which communicates with the editor using
   the LSP protocol over stdin/stdout.
2. The Velociraptor API server.

Since the LSP protocol uses stdin/stdout it must run locally to the
editor. However, to be able to see all custom artifacts on the server,
the LSP server must communicate with the server over the API connection.

This means that you can run the editor remotely but connect to the
main deployment server. Alternative you can run a local Velociraptor
server to serve the actual LSP connections.

### Connecting to a deployment

To connect the LSP server to remove deployment simply create an API
key as described in the docs. You only need a key with the `reader`
role. Store the key somewhere on the local system.

Create a shell script (for example ~/bin/vqllsp) with execute permissions:
```bash
#!/bin/bash

velociraptor --api_config ~/bin/my_api_key.yaml lsp
```

Update your .emacs config as follows:
```
(use-package lsp-mode
  :init
  ; Enable to get syntax highting in VQL
  (setq lsp-semantic-tokens-enable t)

  :config
  (add-to-list 'lsp-language-id-configuration '(".*\\.vql$" . "vql"))
  (lsp-register-client (make-lsp-client
                        :new-connection (lsp-stdio-connection "vqlpls")
                        :activation-fn (lsp-activate-on "vql")
                        :server-id 'vqlpls))

  :commands lsp)
```

### Connecting to a local server

To start a local server, create a datastore directory:

```
velociraptor gui --datastore /path/to/my/datastore
```

This will generate a `server.config.yaml` file in that datastore. You
can use this instead of an API connection key. Create your lsp client
shell script:

```bash
#!/bin/bash

velociraptor --config /path/to/my/datastore/server.config.yaml lsp
```

Then visiting a buffer with a `.vql` extension, the local LSP server
should start and provide editor assistance.
