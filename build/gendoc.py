#!/usr/bin/env python

text = [ x.strip() for x in open("README.md").readlines() ]
text = [ x.replace("\\", "\\\\") for x in text ]
text = [ '"' + x.replace("\"", "\\\"") + '\\n"' for x in text ]
text = " +\n".join(text)
text = """package main

const USAGE = %s
"""%text
open("cmd/takeown/usage.go", "w").write(text)
