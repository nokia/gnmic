
The HTTP target loader can be used to query targets configurations from a remote HTTP server.

It expects a well formatted `application/json` body and a code 200 response.

It supports secure connections, basic authentication using a username and password and/or Oauth2 token based authentication.

<div class="mxgraph" style="max-width:100%;border:1px solid transparent;margin:0 auto; display:block;" data-mxgraph="{&quot;page&quot;:4,&quot;zoom&quot;:1.4,&quot;highlight&quot;:&quot;#0000ff&quot;,&quot;nav&quot;:true,&quot;check-visible-state&quot;:true,&quot;resize&quot;:true,&quot;url&quot;:&quot;https://raw.githubusercontent.com/openconfig/gnmic/diagrams/diagrams/target_discovery.drawio&quot;}"></div>

<script type="text/javascript" src="https://cdn.jsdelivr.net/gh/hellt/drawio-js@main/embed2.js?&fetch=https%3A%2F%2Fraw.githubusercontent.com%2Fkarimra%2Fgnmic%2Fdiagrams%2Ftarget_discovery.drawio" async></script>

#### Configuration

``` yaml
loader:
  type: http
  # resource URL, must include the http(s) schema
  url: 
  # watch interval at which the HTTP endpoint is queried again
  # to determine if a target was added or deleted.
  interval: 60s
  # HTTP request timeout
  timeout: 50s
  # time to wait before the fist HTTP query
  start-delay: 0s
  # tls config
  tls:
    # string, path to the CA certificate file,
    # this will be used to verify the clients certificates when `skip-verify` is false
    ca-file:
    # string, client certificate file.
    cert-file:
    # string, client key file.
    key-file:
    # boolean, if true, the client will not verify the server
    # certificate against the available certificate chain.
    skip-verify: false
  # username to be used with basic authentication
  username:
  # password to be used with basic authentication
  password:
  # token to be used with Oauth2 token based authentication
  token:
  # auth scheme (default is `Bearer`)
  auth-scheme:
  # text template
  template:
  # path to a text template file
  template-file:
  # if true, registers httpLoader prometheus metrics with the provided
  # prometheus registry
  enable-metrics: false
  # list of actions to run on target discovery
  on-add:
  # list of actions to run on target removal
  on-delete:
  # variable dict to pass to actions to be run
  vars:
  # path to variable file, the variables defined will be passed to the actions to be run
  # values in this file will be overwritten by the ones defined in `vars`
  vars-file:
```

#### Targets file format

=== "JSON"
    ```json
    {
        "10.10.10.10": {
            "username": "admin",
            "insecure": true
        },
         "10.10.10.11": {
            "username": "admin",
        },
         "10.10.10.12": {},
         "10.10.10.13": {},
         "10.10.10.14": {}
    }
    ```

Just like the targets in the main configuration file, the missing configuration fields get filled with the global flags,
the ENV variables first, the config file main section next and then the default values.
