// Package base64 provides method for encoding and decoding base64 strings.
//
//  TODO: We really need to allow bytes as a column type to make this useful as
//  otherwise you can't store the binary data in a table, unless the binary data happens to be a
//  utf-8 encoded string.
package base64


// encode translates binary data into a base64 encoded string with padding.
//
// ## Parameters
// - data: Binary data to encode.
//
// ## Examples
//
builtin encode : (data: bytes) => string
builtin decode : (data: bytes) => string
