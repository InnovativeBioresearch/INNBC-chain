<!-- START ALPHA CHANGE 1025 -- update docs workspace README for INNBC -->

# INNBC Chain Docs

This folder contains the MkDocs documentation workspace for INNBC Chain.

## Run With Python

```bash
cd docs
pip install virtualenv
./run.sh
```

The site runs at:

```text
http://127.0.0.1:8000/
```

## Run With Docker

The Docker files in this docs folder are only for serving the documentation site. They are not the removed legacy local validator cluster Docker flow.

```bash
cd docs
docker build -t innbc-chain-docs .
docker compose up
```

The site runs at:

```text
http://127.0.0.1:8000/
```

<!-- FINISH ALPHA CHANGE 1025 -- update docs workspace README for INNBC -->