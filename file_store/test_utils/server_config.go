package test_utils

const SERVER_CONFIG = `
autoexec:
version:
  name: velociraptor
  version: 0.6.4-rc4
  commit: f3264824
  build_time: "2022-04-14T02:23:05+10:00"
Client:
  server_urls:
  - https://localhost:8000/
  ca_certificate: |
    -----BEGIN CERTIFICATE-----
    MIIDTDCCAjSgAwIBAgIRAJH2OrT69FpC7IT3ZeZLmXgwDQYJKoZIhvcNAQELBQAw
    GjEYMBYGA1UEChMPVmVsb2NpcmFwdG9yIENBMB4XDTIxMDQxMzEwNDY1MVoXDTMx
    MDQxMTEwNDY1MVowGjEYMBYGA1UEChMPVmVsb2NpcmFwdG9yIENBMIIBIjANBgkq
    hkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAsLO3/Kq7RAwEhHrbsprrvCsE1rpOMQ6Q
    rJHM+0zZbxXchhrYEvi7W+Wae35ptAJehICmbIHwRhgCF2HSkTvNdVzSL9bUQT3Q
    XANxxXNrMW0grOJwQjFYBl8Bo+nv1CcJN7IF2vWcFpagfVHX2dPysfCwzzYX+Ai6
    OK5MqWwk22TJ5NWtUkH7+bMyS+hQbocr/BwKNWGdRlP/+BuUo6N99bVSXqw3gkz8
    FLYHVAKD2K4KaMlgfQtpgYeLKsebjUtKEub9LzJSgEdEFm2bG76LZPbKSGqBLwbv
    x+bJcn23vb4VJrWtbtB0GMxB1bHLTkWgD6PV6ejArClJPvDc9rDrOwIDAQABo4GM
    MIGJMA4GA1UdDwEB/wQEAwICpDAdBgNVHSUEFjAUBggrBgEFBQcDAQYIKwYBBQUH
    AwIwDwYDVR0TAQH/BAUwAwEB/zAdBgNVHQ4EFgQUO2IRSDwqgkZt5pkXdScs5Bjo
    ULEwKAYDVR0RBCEwH4IdVmVsb2NpcmFwdG9yX2NhLnZlbG9jaWRleC5jb20wDQYJ
    KoZIhvcNAQELBQADggEBABRNDOPkGRp/ScFyS+SUY2etd1xLPXbX6R9zxy5AEIp7
    xEVSBcVnzGWH8Dqm2e4/3ZiV+IS5blrSQCfULwcBcaiiReyWXONRgnOMXKm/1omX
    aP7YUyRKIY+wASKUf4vbi+R1zTpXF4gtFcGDKcsK4uQP84ZtLKHw1qFSQxI7Ptfa
    WEhay5yjJwZoyiZh2JCdzUnuDkx2s9SoKi+CL80zRa2rqwYbr0HMepFZ0t83fIzt
    zNezVulkexf3I4keCaKkoT6nPqGd7SDOLhOQauesz7ECyr4m0yL4EekAsMceUvGi
    xdg66BlldhWSiEBcYmoNn5kmWNhV0AleVItxQkuWwbI=
    -----END CERTIFICATE-----
  nonce: rKNKAYam310=
  writeback_darwin: /etc/velociraptor.writeback.yaml
  writeback_linux: /tmp/velociraptor.writeback.yaml
  writeback_windows: $ProgramFiles\Velociraptor\velociraptor.writeback.yaml
  max_poll: 600
  windows_installer:
    service_name: Velociraptor
    install_path: $ProgramFiles\Velociraptor\Velociraptor.exe
    service_description: Velociraptor service
  darwin_installer:
    service_name: com.velocidex.velociraptor
    install_path: /usr/local/sbin/velociraptor
  version:
    name: velociraptor
    version: 0.6.4-rc4
    commit: f3264824
    build_time: "2022-04-14T02:23:05+10:00"
  max_upload_size: 5242880
  local_buffer:
    memory_size: 52428800
    disk_size: 1073741824
    filename_linux: /var/tmp/Velociraptor_Buffer.bin
    filename_windows: $TEMP/Velociraptor_Buffer.bin
    filename_darwin: /var/tmp/Velociraptor_Buffer.bin
  disable_checkpoints: true
API:
  bind_address: 127.0.0.1
  bind_port: 8001
  bind_scheme: tcp
  pinned_gw_name: GRPC_GW
GUI:
  public_url: https://localhost:8889/app/index.html
  bind_address: 127.0.0.1
  bind_port: 8889
  gw_certificate: |
    -----BEGIN CERTIFICATE-----
    MIIDRDCCAiygAwIBAgIRAP1dus6h2AmCT/vr3RZCTlQwDQYJKoZIhvcNAQELBQAw
    GjEYMBYGA1UEChMPVmVsb2NpcmFwdG9yIENBMCAXDTIzMDQxMzE4MzI1NFoYDzIx
    MjMwMzIwMTgzMjU0WjApMRUwEwYDVQQKEwxWZWxvY2lyYXB0b3IxEDAOBgNVBAMM
    B0dSUENfR1cwggEiMA0GCSqGSIb3DQEBAQUAA4IBDwAwggEKAoIBAQCe6djXOAP6
    vHOlt8u87heZyWMbo/k7teiCK5etclAesdkgcetEtOk91hXhf4/cQyDkH5S9KQem
    CFcYZO7/0Y8B+QpN/6pBRFKx9O/J30sxCKajUIgKrk5Y8bPsAZzz/dbg3DWIRsjS
    hvcIWOhMDvSuAQwaFc1MT+PeRVHiaD+Jk+BmaXTJVWkdaI0NEJ2uK5zvhiwzXtE7
    PbRCoCJ0nOxDe/CtJqB0ns/1+gZ1te4j+ulAVOfrzEGL1w6JP23RXAoopUgZYOn0
    t5rqAtbOrSDp0JXDPsmr5oP3eOoCCp2GGbIhlp2HthxU6ieqqoKtsnWfl6l/SSLl
    9IXUFmL1pFIVAgMBAAGjdDByMA4GA1UdDwEB/wQEAwIFoDAdBgNVHSUEFjAUBggr
    BgEFBQcDAQYIKwYBBQUHAwIwDAYDVR0TAQH/BAIwADAfBgNVHSMEGDAWgBQ7YhFI
    PCqCRm3mmRd1JyzkGOhQsTASBgNVHREECzAJggdHUlBDX0dXMA0GCSqGSIb3DQEB
    CwUAA4IBAQB3b39mSUoucO3fITDupEB93FQkReDpBnSxUN0YJxMsQJXlLDXQvZJb
    CqZ0SL0CyfDRhesvRg5BNWIG9aZ+ZJx3a7fLsdBsCNQt50JZWsC8VhjppiNqyWc6
    WXDtLBqiueVE4UOY+jnWbcDSqVZjJi7NBspAt3HwQapwjdt4TkdcA+p487lu8pvO
    yQCIcGdsA/gT/DEQZySeIuwcEpNj2kvo5G2sSyc/TDVR6Y6krhFIwTTaQT4B/E9+
    kmks/TKbaG+tCsv2YUvlRwwHKKKDIEQLhmWFHmxiHgyE5RVZq8IICOMGTCN1Ln9n
    IJyGAjGF7klkSonrtoLdER1TeBksg6sm
    -----END CERTIFICATE-----
  gw_private_key: |
    -----BEGIN RSA PRIVATE KEY-----
    MIIEogIBAAKCAQEAnunY1zgD+rxzpbfLvO4XmcljG6P5O7XogiuXrXJQHrHZIHHr
    RLTpPdYV4X+P3EMg5B+UvSkHpghXGGTu/9GPAfkKTf+qQURSsfTvyd9LMQimo1CI
    Cq5OWPGz7AGc8/3W4Nw1iEbI0ob3CFjoTA70rgEMGhXNTE/j3kVR4mg/iZPgZml0
    yVVpHWiNDRCdriuc74YsM17ROz20QqAidJzsQ3vwrSagdJ7P9foGdbXuI/rpQFTn
    68xBi9cOiT9t0VwKKKVIGWDp9Lea6gLWzq0g6dCVwz7Jq+aD93jqAgqdhhmyIZad
    h7YcVOonqqqCrbJ1n5epf0ki5fSF1BZi9aRSFQIDAQABAoIBAAYJqH102WHbayFu
    vETvXuIu7p8MOdn07WKUuWyTnUutQiyjZ2by4LHCwo4QxKx/uG4ybPpK5sl+I6D/
    pLz/f0l55tRT1GoqaGHuhnXLEBZK19n4o1KUkNF8TXO4E/iJOnLMqxQEbHjjO9uL
    VTgekVlTHNyY23X8yxGU3KmXgGJ/tnz1DETRSi01Vz6sT4GA3zh+QX+OoASetb8Q
    HaYFWJMRMR9UnLq8ExaeCV7MJ0y9Z9uVxq2Af57+9eZWjBE7/biooxKnIRGRU68J
    HD/x2+p+ZzfvzqTy0Si8wNyd1vDIYYPbOoSt23cpEzBqB7ij4ZgXdirT6qYC4hEW
    FFWx1PkCgYEA02TPs15B5kNc3CaNIolSTZKEaaQuSxFxVuZxNs7dWaol8ebhMYWM
    3AJ0k8dNI1lKSCI/J/BF6VeSmoVNKr3Vs9JqE57SzR4RpcYx6FhcjsKxgOCLtZAN
    bsrJd7YZJQtew4/MZB5QHWMdZicQECLbKCXt/6cRtt7ztLr/TBVshR8CgYEAwHIf
    sgKqVfmONvcCVOrTER/9pf89ukNtH7JacHcs5xE848jEFzNBDwW0vg8Z0rgchQ0O
    Ugx4c88/OHT+JX01beqnY9hMV5Mp1qumpZA2Q/rhOAMVGcncJ+6YqeEnLwIZfNX6
    6xQ7asqtDdpAKkzIreo8PuQiS/UphaNA4+eabksCgYBJfBvvoG6MGxKmvQgG33Gq
    4aoCBz7IfbHGoajtgo/T4Z/7LWVPD7vdp0TbMkcQaLO3y5/kxFOpP/YInRosJ32o
    Wxbg5y8kerVryTAEMuNKBUgrIuOuI/tnbjsG0FiBViiFFvHYQ+lZreDEaAPfeB5z
    IGxRmMRBq9NQGkkxK6ljxQKBgFE+3Qq1/VuWo+eomJ9pE/qi2t79xv2gAa3kCjJ4
    3cgfiulPlRmGVe0Vp5ylm21OtRumy2jwQtoBoNsg6TrChZAGBO0uH+zJAFzU0uIK
    5B4HCJYxFvNwOTXSkTkHCRfbdw8w92HPhNYtAqpafcRd7kseHJkgjyoqMoFszrRo
    ztXJAoGAaovW+JrgG+7xhVE2Ha1cMBhGcZ22RXM6/U7B7UgLF7jE0zZ0Z83sv6NS
    W5gJrneEd85yOMZRQ+zR9lUBnfK+csnfdPys6Tf+lnTBlXGtXaN69rmAHD1WJp5l
    JL5WubPDAGJoNCt7TqNBOwMk6avXZPFQkVljQclVoysIBQ44Tac=
    -----END RSA PRIVATE KEY-----
  authenticator:
    type: Basic
CA:
  private_key: |
    -----BEGIN RSA PRIVATE KEY-----
    MIIEowIBAAKCAQEAsLO3/Kq7RAwEhHrbsprrvCsE1rpOMQ6QrJHM+0zZbxXchhrY
    Evi7W+Wae35ptAJehICmbIHwRhgCF2HSkTvNdVzSL9bUQT3QXANxxXNrMW0grOJw
    QjFYBl8Bo+nv1CcJN7IF2vWcFpagfVHX2dPysfCwzzYX+Ai6OK5MqWwk22TJ5NWt
    UkH7+bMyS+hQbocr/BwKNWGdRlP/+BuUo6N99bVSXqw3gkz8FLYHVAKD2K4KaMlg
    fQtpgYeLKsebjUtKEub9LzJSgEdEFm2bG76LZPbKSGqBLwbvx+bJcn23vb4VJrWt
    btB0GMxB1bHLTkWgD6PV6ejArClJPvDc9rDrOwIDAQABAoIBAAo6vUIBWEn+MBzD
    SAi080S3cNZFftVUNIfpAObjcgr+Rv/0eeHPSHlvd1wC23eyU2p0UC4j75b/OM/F
    t/z0a1aKAxkF5M/KFk/dWy7FGcWIvcWEbl9GoAPuaBfnKR0tDVmOEsy0P08HdU8L
    9+UCYiBvAK1eQlD3oGA7pvB/9DpHKLSiZOBtmss0EXuJdixKvlcF6GPHBpAjG90g
    ogwcRXJt8qJm9/N5pz+3odYFttXwBn7bdxNLBaUkG3RvrFHUslmN7V0tvFIpjAIT
    f7/5jmLhJugoP6wl9hUEsUSrcdRmSYKRNuHFU06OazBTlka4ksM3z2RFJ6TRhxXZ
    s8U8o3ECgYEAwYKeDJQcx+gRC26Vq6EWT5oHZOLrTh5QrZv/cBo0YP8nhLR0uzwz
    HNj8sMgyFV8yLCYvWaqgRCfCwMoMAUQCH5q0GPNxlQuaL+3WjcTwQeTPms9IuMFh
    rTDt1mi3xPwc5n8ZNafB8+1cNJKOCvrKXdxM/kmRIJVUaFREjyM+LgUCgYEA6cOT
    sl2fp80n10VONcFeVIEaN+YjBapDBJzaNThxTVzjBRsPyUzgEIhQ6r6V8LmG56Wo
    VfyELuvNHgKYvA6mIlsH6l3SLq+F7ohwEDVikp0yzjiMRRhhxQUsnahtHhX3JsUd
    yX2hQOLaaNfNV7gYx64a4iWizFrEa9J2wSUQuD8CgYEAmHZD9h8gCfTysPIg5EeX
    34G4/6i1wieqYw58lCNhT2bZCPpw2jBVCQ6BEPu6UhJd4mD3f4sqmGhHTkQib0DY
    93OZH+t2evrYMZkPKUWYEiKn2w4j+sUKIz1gtkRtPbtxPb237AlPi9NgiV9KoKX1
    mTwAQX1O5cAh780s8yXOUM0CgYA/zC6c+Uw/YZBEAhgsN4/lBC8Bnn9kZmlP8vbi
    m3rgoD8c/5u5Vo+4M1vSFR2ayyd0RRPCE96HZ7ddP1wrxtu0eJ+aaOyZ7TFiPj5H
    TiqO1PQur+QoX1Ufjh/1Dyhok5oWLKnKeczuhnsRLgROsmGg7XVMzvS1TPhabOAY
    KmN7xQKBgEnOjlbCT24fvolHxSJETuoq5IHjwnB/DKTMfnsFfqDPgC/rljqQMF5v
    yzPC/h0xqCh/dI7pIsJ5FjEXOtIJT/sWa1iddB7WC2oFh6AIrVJszt0dQx+4lS2m
    OgdvbViAVYsGELhg/EeJs/ig1v27BMcv2aQtZXTEHXmOd2xL93l5
    -----END RSA PRIVATE KEY-----
Frontend:
  hostname: localhost
  bind_address: 0.0.0.0
  bind_port: 8000
  certificate: |
    -----BEGIN CERTIFICATE-----
    MIIDWTCCAkGgAwIBAgIQcyUFy1oMUr4O4sIOhom/jDANBgkqhkiG9w0BAQsFADAa
    MRgwFgYDVQQKEw9WZWxvY2lyYXB0b3IgQ0EwIBcNMjMwNDEzMTgzMjUzWhgPMjEy
    MzAzMjAxODMyNTNaMDQxFTATBgNVBAoTDFZlbG9jaXJhcHRvcjEbMBkGA1UEAxMS
    VmVsb2NpcmFwdG9yU2VydmVyMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKC
    AQEA9MSMbrFjmZs9bnpkel4vTQIyf+6Bpg60ByC7d6WWfBwvHdF1Qnfn1JO3Xo6p
    53I1jPoagt0cZCzd6nwJXJ/3pclprmIOEBSc20pg5E0A/kpwn+bBoPNSrMF7+2/t
    DvXP0Lvs/1OqUMjF8pCs6vnSKigaptn+0Et3GpzWjwCghqPcJBOuEuPQmR3HyHfs
    dsMooCjuYcRcS9MXioT97SSjxeug0oTXHaKCnQ7txoxuN2+nNdr03mUu07TOUbRp
    X3NsiaoESl/9IDC/tz2XTBD3UxLze9pX9t4tdKEMK2+gdnrnioOw1D7WBoElECj9
    +89CRXlu3K15P1cNVB5htPzOgwIDAQABo38wfTAOBgNVHQ8BAf8EBAMCBaAwHQYD
    VR0lBBYwFAYIKwYBBQUHAwEGCCsGAQUFBwMCMAwGA1UdEwEB/wQCMAAwHwYDVR0j
    BBgwFoAUO2IRSDwqgkZt5pkXdScs5BjoULEwHQYDVR0RBBYwFIISVmVsb2NpcmFw
    dG9yU2VydmVyMA0GCSqGSIb3DQEBCwUAA4IBAQAhwcTMIdHqeR3FXOUREGjkjzC9
    vz+hPdXB6w9CMYDOAsmQojuo09h84xt7jD0iqs/K1WJpLSNV3FG5C0TQXa3PD1l3
    SsD5p4FfuqFACbPkm/oy+NA7E/0BZazC7iaZYjQw7a8FUx/P+eKo1S7z7Iq8HfmJ
    yus5NlnoLmqb/3nZ7DyRWSo9HApmMdNjB6oJWrupSJajsw4Lsos2aJjkfzkg82W7
    aGSh9S6Icn1f78BAjJVLv1QBNlb+yGOhrcUWQHERPEpkb1oZJwkVVE1XCZ1C4tVj
    PtlBbpcpPHB/R5elxfo+We6vmC8+8XBlNPFFp8LAAile4uQPVQjqy7k/MZ4W
    -----END CERTIFICATE-----
  private_key: |
    -----BEGIN RSA PRIVATE KEY-----
    MIIEpAIBAAKCAQEA9MSMbrFjmZs9bnpkel4vTQIyf+6Bpg60ByC7d6WWfBwvHdF1
    Qnfn1JO3Xo6p53I1jPoagt0cZCzd6nwJXJ/3pclprmIOEBSc20pg5E0A/kpwn+bB
    oPNSrMF7+2/tDvXP0Lvs/1OqUMjF8pCs6vnSKigaptn+0Et3GpzWjwCghqPcJBOu
    EuPQmR3HyHfsdsMooCjuYcRcS9MXioT97SSjxeug0oTXHaKCnQ7txoxuN2+nNdr0
    3mUu07TOUbRpX3NsiaoESl/9IDC/tz2XTBD3UxLze9pX9t4tdKEMK2+gdnrnioOw
    1D7WBoElECj9+89CRXlu3K15P1cNVB5htPzOgwIDAQABAoIBAGAAy3gLOZ6hBgpU
    FR7t3C2fRAFrogxozfHRw9Xc69ZIE67lXdGxSAvX2F9NI5T09c4Stt1HLoCYHH6B
    Igbjc3XiNwI/0XY7L37PgItrLI2Q0vXUw3OGnJHH3gIz10472cPsQbuvrCi9Zu6K
    ElijnewNCM8Sx+AZCWE1zO4P9+Z2kF9LvWzDwAa643jQ/Dg+S68zCFqjJCVJBGm+
    LQxDs6dbArvOiEbuZs2wDt0d1kZF+BRljUTMoCpdf3jmFj3f0Jc1AFaz1eHG9Gte
    XIUpbWmV2ATABSW2kDkVdXx+m/w1r9PZCLLfq54fIOlm2IeAiM3rDmM4ZSTUYEPn
    mJP03xECgYEA+jS7DiS3bB/MeD+5qsgS07qJhOrX17s/SlamC1dQqz+koJLl98JX
    CqyafFmdSz7PK2S2+OOazngwx26Kc3MZFoD9IQ2tuWmwDgbY8EQs5Cs37By2YRZJ
    DdjvVf48pCKiXxIhvFjW/5CTemNAAu4CXg5Lkp7UVVrOmf5BmjMmE0sCgYEA+m+U
    QMF0f7KLM4MU81yAMJdG4Sq4s9i4RmXes2FOUd4UoG7vEpycMKkmEaqiUVmRHPjp
    P6Dwq3CK+FVFMpCeWjn6KkxwpdWWO9lglI0npFcPNW/PzPOv4mSNtCAcpHrKFP0R
    3jbc8UhgtFxDZoeUih7cO2iTO7kELBCeKUzw9qkCgYBgVYcj1e0tWzztm5OP9sKQ
    9MRYAdei/zxKEfySZ0bu+G0ZShXzA8dhm71LXXGbdA5t5bQxNej3z/zv/FagRtOE
    /5r2a/7UYaXgcLB8KbOjEiTQ6ukpjlwIUdssn9uXUqJzulZ03zvAYFj4CVivCBav
    Qg/E3xRf3LupPOTjSwhA6wKBgQDAH3tnlkHueSWLNiOLc0owfM12jhS2fCsabqpD
    iQHRkoLWdWRZLeYw+oLnCLWPnRvTUy11j90yWJt0Wc5FNWcWJuZBLvU4c7vWXDRY
    olVoIRXc09NiEwy6rJN9PSlcEYsYQPFFPWeQfwsZMrLOZHLS50vjE53oMk7+Ex2S
    56DwSQKBgQC+iHbsbxloZjVMy01V21Sh9RwIpYrodEmwlTZf2jzaYloPadHu4MX1
    jHG+zzeC/EJ3wFOKTSJ/Tmjo6N3Xaq9V7WeL8eBdtBtPztqN1yveTt94mZZ+fuID
    BhI8P2RbNR2Yey5nnhFQcoTxpmVw3EYwE01nkxoPJRs/QVvxi9Mepg==
    -----END RSA PRIVATE KEY-----
  dyn_dns: {}
  default_client_monitoring_artifacts:
  - Generic.Client.Stats
  GRPC_pool_max_size: 100
  GRPC_pool_max_wait: 60
  resources:
    connections_per_second: 100
    notifications_per_second: 10
    max_upload_size: 10485760
    expected_clients: 10000
    default_log_batch_time: 100
    default_monitoring_log_batch_time: 100
    disable_file_buffering: true

Datastore:
  implementation: Test
  location: /tmp
Writeback:
  private_key: |
    -----BEGIN RSA PRIVATE KEY-----
    MIIEogIBAAKCAQEArmgftoc6pi/ZMGZO40UIKXlscTXrZWifDtTGsAhXfaKG4xzu
    LLLIM4Cr+L3ctYgFkWyczXst6Tx6zRyU/l2OqaWmJjhNwXlRwNajx+2ZqTa5zA8r
    lr+QeYrg19+Acmgb8DkPwp8in/f3tHl7Na8U8GE/3CX4nMsLOzcfAEdH/4IRh3b0
    3VW361dlBL8Sw2KJ7ECmhujjtlxu7BUDolxxf8bIkFDVt/nhs9xxm2yI+b2xQnsy
    LDHpsZzSuXj/M38s8u0r59QtJ+ByjFjte+gjGpTc9WlMytTvI/RJUbiEKwOPjBVn
    BcV/1IZ08KokSfhq4xpVY/GPZVL4CEf/ZOo9rQIDAQABAoIBAFnNUW75yHAjuRBb
    zYjmVaKNXBIa8l8f9K59TuT7FpmhIxU0I0surzkdqu8ES+3I4R0VMNP49hXfR1fv
    vKQQ5lFh8uBBI4BYiIjjvCdIp1Ni015H/Wi8sJZ0tPtSoN/HzYLuzremmvyFgK0T
    1CY7RWvUlz4y6wVI4zqVUkgha+gaZjoQklzamwqKHQwqtFyPVISmSp6XL/zexk95
    GVUGps4mtsXWUsSnsmlUD5Ola/7hXeEgbD1nj2Znobu9z0y8mDlIhpFpQJwu36KB
    3o3tqBOXuoukxmsvuW8QxW1xzCICuh8CU5g6kWkyNJOsf4X5Y2js/5Zo9dbLkOrd
    VEnnV+ECgYEAydqTmbWQyOeUxV9mfD2BbjnzLvxMCCggW/i4TGYhtLweO7UPSiQT
    /zK0KX317vupUou//vGEcFKLPVu4xchsGrayOVCWEpurqvZfPmg9lWyF2fi8rZK0
    vOWCw8HIgIbb8EvRCH1v0gNMdzjaf1qLN28W5H/7re4rruQOEuyv29kCgYEA3TC9
    XFAVSePV/Ky22AdbccVacABmM5RAneot/E7DTrA9uGujUB+9kCPIDsPLCjT2uXj/
    yP/a210t8KZBtvW+1Ums06titw65lkG7rjapB08vjF1aD0bjPE4R1uapm+CM6dlm
    oc3Beb8kyA+bXZMpnJT1KtAI3/zrdlZkhQlAL/UCgYAs/uViIUAqGL1oFfERhuBg
    Qti7w4/rTY6REet7VFT1Je4TXzQOUeaHP7U7fpGg+UZwWSiuWwYrx6q0Pcr9g8Td
    W5Z1AkrB0SO+U3c9wRzhPzTDNxhQFODnLr4shvj79ZP3h98L5nJTvVqBRRIny3Y3
    IDNZMlJXHj1smfetLkexWQKBgBgcgAfYEvoDBAiPKz9RTf6Q7NLYuEtXFdQg+vJO
    A6xIOfIoiZzqWNeljuFNJozuSRbewcM/YLQY7DEXboJrN2o4pcZNIG2kBUcD01mi
    S7qoPx6l7nNL3ulr+TXb3xFG4RV8xVtN+pEy7OeCDAWfTSHseu030D/aajB0KnD2
    GTEhAoGARB/E6j/WX+CBPWiF4XLV03F1hEMYY/ZSfijcZQniCNtRQUuIkTMiSv1E
    LZ5KmiY35bmYwkGOST6sd9T586nNEdIfs2ngcXwRcgPmQU7VaKQdeVnxhEG2xXFG
    NtyI/STijkpVi99wF39BvXkQGdJuDjAArjGj5kevCpvyveudL5g=
    -----END RSA PRIVATE KEY-----
Mail: {}
Logging:
  debug: {}
Monitoring:
  bind_address: 127.0.0.1
  bind_port: 8003
api_config: {}
obfuscation_nonce: RzlAlmdcUyw=
defaults:
  hunt_expiry_hours: 168
  notebook_cell_timeout_min: 10
  backup_period_seconds: -1

services:
  hunt_manager: false
  hunt_dispatcher: false
  stats_collector: false
  server_monitoring: false
  server_artifacts: false
  dyn_dns: false
  interrogation: false
  sanity_checker: false
  vfs_service: false
  user_manager: true
  client_monitoring: false
  monitoring_service: false
  api_server: false
  frontend_server: true
  gui_server: false
  index_server: true
  journal_service: true
  notification_service: true
  repository_manager: false
  test_repository_manager: true
  inventory_service: true
  client_info: true
  label: true
  launcher: true
  notebook_service: false

`
