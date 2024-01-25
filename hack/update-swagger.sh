#!/bin/bash

export SWAGGER_VERSION="5.11.0"

wget -O swagger-ui.zip https://github.com/swagger-api/swagger-ui/archive/refs/tags/v$SWAGGER_VERSION.zip -q
rm -rf swagger-ui && mkdir swagger-ui-tmp && mkdir swagger-ui
unzip swagger-ui.zip "swagger-ui-$SWAGGER_VERSION/dist/*" -d "swagger-ui-tmp"
mv swagger-ui-tmp/swagger-ui-$SWAGGER_VERSION/dist/* swagger-ui/
rm -rf swagger-ui-tmp swagger-ui.zip
sed -i 's/https:\/\/petstore.swagger.io\/v2\///g' swagger-ui/swagger-initializer.js
cat <<EOF > swagger-ui/swagger.go
package swagger

import (
    "embed"
)

//go:embed *
var Content embed.FS
EOF
