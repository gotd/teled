# teled

WIP Telegram Server in Go.

## Using

Documentation is in progress.
Later the automated patch utility will be provided.

Do not use Telegram name or branding for custom clients.

## Private keys
Generate new RSA private key and save armored keys to some file.
You will need to vendor public keys to your clients.

### tdesktop

Update following files:
* Telegram/SourceFiles/mtproto/details/mtproto_domain_resolver.cpp
* Telegram/SourceFiles/mtproto/mtp_instance.cpp
* Telegram/SourceFiles/mtproto/mtp_instance.h
* Telegram/SourceFiles/mtproto/mtproto_dc_options.cpp
