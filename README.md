# SPIR-V code generation and embedding for Golang

This is a tool for easily compiling GLSL source files into SPIR-V modules and
embedding them into Go source files along with useful metadata. I created it
primarily for my own use so that editing shaders requires less changes elsewhere
in the code, but feel free to use it if you find yourself having the same niche
needs as me.

This tool avoids compiling unchanged code and will react to new and deleted
source files accordingly. Binary SPIR-V data is accessed as []uint32.

## Usage:

`spv [[options]]`

| Option   | Description | Argument | Required |
| -------- | --------- | -------- | ----------- |
| -pkg     | Name of the output package | string | &#10003; |
| -args    | Arguments for the compiler as a string | string | |
| -dir     | Path to the directory with the GLSL source files | string | |
| -force   | Force shader file re-compilation | | |
| -cc      | GLSL compiler to use (default: glslangValidator) | string | |
| -verbose | Self-explanatory | | |

## License

This software is licensed under GNU GPLv2. You are free to license generated
code however you like.
