// Package scala reads Scala .scl scale files and .kbm keyboard mappings and writes .kbm files.
//
// Parsing is lenient: it keeps exact ratios and reports problems as warnings
// (non-ascending degrees, count mismatches) instead of failing where possible.
// It never writes .scl files.
package scala
