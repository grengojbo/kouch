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
    body="
## Testing

This is a test

# a subhead

more test
"
    printf %q "$body"
    ;;
    name)
        echo "A cool name"
    ;;
esac
