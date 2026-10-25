#!/usr/bin/env bash
timeout 120 wine "${@}" > >(grep -v 'MESA\|wgl\|fixme\|actctx') 2>&1 </dev/null
