# Security policy

## How to report a vulnerability

Use the repository's private vulnerability reporting (the *Security* tab, *Report a vulnerability* button). If you cannot use it, write to contato@quantogasta.com.br with the subject "security". Do not open a public issue for a security flaw.

Describe what happens, how to reproduce it and what the effect is. I reply within 7 days.

## Never send credentials

Do not send a password, `client_secret`, token or any real credential, whether in a report, an issue or an attachment. If a credential leaked, rotate it with the provider before contacting me. Postman collection exports are not welcome either: they often carry credentials.

## Scope

This covers the content of this repository: the spec, the mock, the Go SDK and the scripts. Flaws in the third-party API that the spec describes should be reported to whoever is responsible for it.
