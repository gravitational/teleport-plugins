<source>
    @type http
    port 8888

    <transport tls>
        client_cert_auth true
        ca_path {{index .CaPaths 0}}
        cert_path {{index .ServerPaths 0}}
        private_key_path {{index .ServerPaths 1}}
        private_key_passphrase "{{.Pwd}}"
    </transport>

    <parse>
      @type json
      json_parser oj

      # This time format is used by Go marshaller
      time_type string
      time_format %Y-%m-%dT%H:%M:%S
    </parse>
</source>

<match test.log>
  @type stdout
</match>