package dict


// dict.fromList is a function that converts an array of key/value pairs
// into a dictionary.
//
// - `pairs` is the list of records that each contain key and value properties.
//
builtin fromList : (pairs: [{key: K, value: V}]) => [K:V] where K: Comparable

// dict.get is a function that retrieves the value of a specified key from
// a dictionary. If the key does not exist in the dictionary, the default
// value will be returned.
//
// - `dict` is the dictionary to return a value from.
// - `key` is the key to return from the dictionary.
// - `default` is the default value to return if the given key does not
//	exist. Must be the same type as values in the dictionary.
//
builtin get : (dict: [K:V], key: K, default: V) => V where K: Comparable

// dict.insert is a function that inserts a key/value pair into the dictionary
// and return a new dictionary with that value inserted.
// If the key already exists in the dictionary, it will
// be overwritten.
//
// - `dict` is the dictionary to update.
// - `key` is the key to insert into the dictionary. Must be the same
//	type as the existing keys in the dictionary.
// - `value` is the value to be inserted into the dictionary. Must be
//	the same type as the existing values in the dictionary.
//
builtin insert : (dict: [K:V], key: K, value: V) => [K:V] where K: Comparable

// dict.remove is a function that removes a key/value pair from the dictionary
// and return a new, updated dictionary.
//
// - `dict` is the dictionary to remove the key/value pair from.
// - `key` is the key to remove from the dictionary.
//
builtin remove : (dict: [K:V], key: K) => [K:V] where K: Comparable
