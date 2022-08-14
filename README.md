# teled

WIP Telegram Server in Go.

Able to handle key exchange and establish connection:
```
 INFO    cmd/root.go:50  Listening       {"addr": "localhost:10443", "dc": 1}
 INFO    tgtest/server.go:92     Serving
DEBUG   tgtest/loop.go:40       User connected
DEBUG   tgtest/loop.go:73       Starting key exchange
DEBUG   exchange        exchange/server_flow.go:110     Received client ReqPqMultiRequest
DEBUG   exchange        exchange/server_flow.go:126     Sending ResPQ   {"pq": "1724114033281923457"}
DEBUG   exchange        exchange/server_flow.go:151     Received ReqPQ again
DEBUG   exchange        exchange/server_flow.go:126     Sending ResPQ   {"pq": "1724114033281923457"}
DEBUG   exchange        exchange/server_flow.go:155     Received client ReqDHParamsRequest
DEBUG   exchange        exchange/server_flow.go:220     Sending ServerDHParamsOk        {"g": 3}
DEBUG   exchange        exchange/server_flow.go:234     Received client SetClientDHParamsRequest
DEBUG   exchange        exchange/server_flow.go:259     Sending DhGenOk
DEBUG   tgtest/loop.go:73       Starting key exchange
DEBUG   exchange        exchange/server_flow.go:110     Received client ReqPqMultiRequest
DEBUG   exchange        exchange/server_flow.go:126     Sending ResPQ   {"pq": "1724114033281923457"}
DEBUG   exchange        exchange/server_flow.go:155     Received client ReqDHParamsRequest
DEBUG   exchange        exchange/server_flow.go:220     Sending ServerDHParamsOk        {"g": 3}
DEBUG   exchange        exchange/server_flow.go:234     Received client SetClientDHParamsRequest
DEBUG   exchange        exchange/server_flow.go:259     Sending DhGenOk
DEBUG   tgtest/handle.go:40     Send handleSessionCreated event {"session_id": -5370167338796072529, "key_id": "37c12945217bbf85"}
DEBUG   tgtest/handle.go:70     Got request     {"session_id": -5370167338796072529, "key_id": "37c12945217bbf85", "msg_id": 7131794985135297912, "type": "message_container"}
DEBUG   tgtest/handle.go:70     Got request     {"session_id": -5370167338796072529, "key_id": "37c12945217bbf85", "msg_id": 7131794985134798188, "type": "auth.bindTempAuthKey#cdd42a05"}
DEBUG   tgtest/handle.go:70     Got request     {"session_id": -5370167338796072529, "key_id": "37c12945217bbf85", "msg_id": 7131794985135247744, "type": "ping_delay_disconnect#f3427b8c"}
DEBUG   tgtest/loop.go:40       User connected
```

## Using

Documentation is in progress.
Later the automated patch utility will be provided.

Do not use Telegram name or branding for custom clients.

## Private keys
Generate new RSA private key and save armored keys to some file.
You will need to vendor public keys to your clients.

### Testing key

```pem
-----BEGIN RSA PRIVATE KEY-----
MIIEpQIBAAKCAQEAwX3W9oxq8FdwkMOA/HNeeDJOx8pF6fIw4MrtbfsYPhc5yTs0
lg6H3ia4SmUTnpbJ+NhGxQX2nhDR6911SETBIq1Iz0OUuOPyDVhVhyZzF+vzBukU
E75slPy3UzAzAyruOT5iCoX7mCDhRHWh9Rhm2HHVC02LAKM7RYx6u3WyjI4LV1a1
FplaD0wTh1ElJQcHkRsCVJzfD2fCSrkO/F2QNuOdEe9uoyIFGZjTX82GJNd/H6DX
KB4XQ158yJifZBwZohH9q/7IYrTEGUreQyMjeL1CiROwYKSEmHOaDnLWqq4u8KSm
jhqNeQnhbTFuwG958vSX1zJOqkDkXlue/1XeMQIDAQABAoIBAQCzEzIV8JMwaNyn
Pilse6HdAEJlKdFW2W1fPrBLb25aWMiEcyUSE9SvR4qcX7uutMOsaQ3mXsSGOe7u
qoFXJzrQSyvtlxBXpa9ppm1RrcYrR5YeuSx5sW1w3gsVFLDBp8Peetvl/WaCvzt9
Tplb6v+UJpYT5epV5rx+e6tDP4TGCzr8aWqM+dA1Pz6eahu9YgRQCUrKogRGqKoM
187Mf5r7IpM0YdXgSnFQ8Pk2+JGYuqR8cjbf+KO/NSfxDWrzNY5jtz1sB7oUhzSV
5ySV1cLNNvrSViiZ/lMjaNd5IclLsJogKdQX6zFJY4uZUuvXi7WY4mJMAudRjcMl
HpQtLAABAoGBAMxDwCfWhGvR3KTl9N4AxmzQfSru7leq+DKFGlwqiQAmCm8SHPsH
ZMLUgOy1nVIP4ZsmSuPCou36CI4EVLHg0ODIeLpp5XHxcnITGNaTajji2ZBKiMDN
n9O7bnHEwUjR0onAPBG13QCA6YW4qa08Wdrc/hqfatjMMpxK8BzBaBYxAoGBAPJ/
k4RrRkPhlED21Y/qmWIYEgHkIR2yF4ykbeJEXEjX3w2sZolgxFFF0W6ldCDdK7li
Baa9buwsA+DP8UDimkkCf5rQju7qvbx9a/xwPfuQuWnc46zt7W4k7o5k64KqIJPw
s7mZaihwZz6JaOozfz4tSUskbTM/HjUWbR2WsEgBAoGBAJRkp+WJL3yQ4qWdNc5O
a9jDghs9p0NjPpqdHfBVKmBEQpI8a3dnAKmV7e+JZTgnt3OaVw+t5+XRHoPl2426
UKTsnuB2bCziBo2fGA1S3PlkvD/aFg1TlMgiQ3M9SFnZrQVL9Ze8MkjaXkw6QnJL
BNA+eg/nPp0vg5kNy/BoBXERAoGBALJ4dDBL503EMqFRSMH/jd1lC7O6myjIzo4i
4gBaAXVW8wGUNW67+iA6ezWsrXgWbrykSLZ7yqwkhMIrKEpxu80p+pINFnT97KBR
ymziiqufyuX/vMyj209p/Oxtl+r1nL5ks1FQeJHEkCe1Z7KeLfKrz7pu17OUq320
wJ+7LIgBAoGAS3P7wby9AE2xE2Pzd1mf5QsFT6dSqjt4ssipU7FvZ6jq8BEjPeu+
bZNkMn6gMDQ8rJsCcyCESPC9TeXTOmuWl93DmybtZA2QiSUxwxogpib1+Zg2UsUE
6+9RS7FVAqgGztFKAS3180Pz62kvgcxvYiIwLNoD2k+A/cNRia+9H3I=
-----END RSA PRIVATE KEY-----
```

```cpp
const char *kPublicRSAKeys[] = { "\
-----BEGIN RSA PUBLIC KEY-----\n\
MIIBCgKCAQEAwX3W9oxq8FdwkMOA/HNeeDJOx8pF6fIw4MrtbfsYPhc5yTs0lg6H\n\
3ia4SmUTnpbJ+NhGxQX2nhDR6911SETBIq1Iz0OUuOPyDVhVhyZzF+vzBukUE75s\n\
lPy3UzAzAyruOT5iCoX7mCDhRHWh9Rhm2HHVC02LAKM7RYx6u3WyjI4LV1a1Fpla\n\
D0wTh1ElJQcHkRsCVJzfD2fCSrkO/F2QNuOdEe9uoyIFGZjTX82GJNd/H6DXKB4X\n\
Q158yJifZBwZohH9q/7IYrTEGUreQyMjeL1CiROwYKSEmHOaDnLWqq4u8KSmjhqN\n\
eQnhbTFuwG958vSX1zJOqkDkXlue/1XeMQIDAQAB\n\
-----END RSA PUBLIC KEY-----" };
```

```console
go install ./cmd/teled
teled --key _testdata/test.key.pem
```

### tdesktop

Update following files:
* Telegram/SourceFiles/mtproto/details/mtproto_domain_resolver.cpp
* Telegram/SourceFiles/mtproto/mtp_instance.cpp
* Telegram/SourceFiles/mtproto/mtp_instance.h
* Telegram/SourceFiles/mtproto/mtproto_dc_options.cpp

Instead, you can use [gotd/tdesktop](https://github.com/gotd/tdesktop) fork.

#### Building docker image for fork
```console
pip install poetry

git clone --recursive https://github.com/gotd/tdesktop.git
cd tdesktop/Telegram/build/docker/centos_env
poetry install
poetry run gen_dockerfile | docker build -t tdesktop:centos_env -
```

#### Building fork
From tdesktop root directory:
```bash
docker run --rm -it \
    -v $PWD:/usr/src/tdesktop \
    -e DEBUG=1 \
    tdesktop:centos_env \
    /usr/src/tdesktop/Telegram/build/docker/centos_env/build.sh \
    -D TDESKTOP_API_ID=17349 \
    -D TDESKTOP_API_HASH=344583e45741c457fe1862106095a5eb \
    -D DESKTOP_APP_USE_PACKAGED=OFF \
```

