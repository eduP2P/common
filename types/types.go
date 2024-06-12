// Package types is a super-package that contains all library code that toversok would need to interact with.
// Such as data layer parsing, higher-level types such as relay information, and more.
//
// This package exists to avoid import cycles, and to clean up all misc/"leaf" functions and types into one hierarchy.
//
// As a general rule to avoid import cycles inside this package:
//   - Only import parent packages, don't import child packages
//   - Importing from a "sibling" package (up the tree) is allowed.
package types
