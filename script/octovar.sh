#!/bin/bash
set -e

case "$1" in
    prerelease)
        if [[ "${TRAVIS_TAG}" == *"-"* ]]; then
            echo true
        else
            echo false
        fi
    ;;
    body)
    echo <<EOF
## Testing

This is a test

# a subhead

more test
EOF

    ;;
    name)
        echo "A cool name"
    ;;
esac
