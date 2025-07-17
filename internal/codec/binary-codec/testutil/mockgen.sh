#!/usr/bin/env bash

mockgen -source=types/interfaces/binary_parser.go -destination=types/testutil/binary_parser_mock.go -package=testutil
mockgen -source=serdes/interfaces/definitions.go -destination=serdes/testutil/definitions_mock.go -package=testutil
mockgen -source=serdes/interfaces/field_id_codec.go -destination=serdes/testutil/field_id_codec_mock.go -package=testutil