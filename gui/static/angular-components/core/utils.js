'use strict';

goog.module('grrUi.core.utils');
goog.module.declareLegacyNamespace();

var debug = false;

/**
 * Regex that matches client ids.
 *
 * @const
 * @export
 */
exports.CLIENT_ID_RE = /^C\.[0-9a-fA-F]{16}$/;


/**
 * Converts camelCaseStrings to dash-delimited-strings.
 *
 * Examples:
 * "someTestInput" -> "some-test-input"
 * "some string with spaces" -> "some-string-with-spaces"
 * "some string with $ symbols" -> "some-string-with-symbols"
 * "someDDirectiveName" -> "some-d-directive-name"
 * "someDDDirectiveName" -> "some-d-d-directive-name"
 * "SOMEUppercaseString" -> "s-o-m-e-uppercase-string"
 *
 * @param {string} input String to be converted.
 * @return {string} Converted string.
 */
exports.camelCaseToDashDelimited = function(input) {
  return input.replace(/\W+/g, '-')
      .replace(/([A-Z])/g, '-$1')
      .replace(/^-+/, '')  // Removing leading '-'.
      .replace(/-+$/, '')  // Removing trailing '-'.
      .toLowerCase();
};


/**
 * Converts the given uppercase string to title case - capitalizes the first
 * letter, converts other letters to lowercase, replaces underscores with
 * spaces.
 *
 * Eg "CONSTANT_NAME" -> "Constant name"
 *
 * @param {string} input Uppercase string to be converted.
 * @return {string} Converted string.
 */
exports.upperCaseToTitleCase = function(input) {
  return (input.charAt(0).toUpperCase() +
          input.slice(1).toLowerCase()).replace(/_/g, ' ');
};


/**
 * Splits comma-separated string into a list of strings. Trims every string
 * along the way.
 *
 * @param {string} input Comma-separated string.
 * @return {Array<string>} List of trimmed strings.
 */
exports.stringToList = function(input) {
  var result = [];

  angular.forEach((input || '').split(','), function(item) {
    item = item.trim();
    if (item) {
      result.push(item);
    }
  });

  return result;
};


/**
 * Strips 'aff4:/' prefix from the string.
 *
 * @param {string} input
 * @return {string} String without 'aff4:/' prefix.
 */
exports.stripAff4Prefix = function(input) {
  var aff4Prefix = 'aff4:/';
  if (input.toLowerCase().indexOf(aff4Prefix) == 0) {
    return input.substr(aff4Prefix.length);
  } else {
    return input;
  }
};


/**
 * Gets last path component (components are separated by '/') from the
 * string.
 *
 * @param {string} input
 * @return {string} The last path component of the input.
 */
exports.getLastPathComponent = function(input) {
  var components = input.split('/');
  return components[components.length - 1];
};


exports.debug = function(name,  item) {
  if (debug) {
    console.log(name);
    console.log(angular.copy(item));
  }
};

exports.PathJoin = function(path, component) {
    return path + '/"' + component.replace(/"/g, '""') + '"';
};

exports.ConsumeComponent = function(path) {
    if (path.length == 0) {
        return {next_path: "", component: ""};
    }

    if (path[0] == '/' || path[0] == '\\') {
        return {next_path: path.substr(1, path.length), component: ""};
    }

    if (path[0] == '"') {
        var result = "";
        for (var i=1; i<path.length; i++) {
            if (path[i] == '"') {
                if (i >= path.length-1) {
                    return {next_path: "", component: result};
                }

                var next_char = path[i+1];
                if (next_char == '"') { // Double quoted quote
                    result += next_char;
                    i += 1;

                } else if(next_char == '/' || next_char == '\\') {
                    return {next_path: path.substr(i+1, path.length),
                            component: result};

                } else {
                    // Should never happen, " followed by *
                    result += next_char;
                }

            } else {
                result += path[i];
            }
        }

        // If we get here it is unterminated (e.g. '"foo<EOF>')
        return {next_path: "", component: result};

    } else {
        for (var i = 0; i < path.length; i++) {
            if (path[i] == '/' || path[i] == '\\') {
                return {next_path: path.substr(i, path.length),
                        component: path.substr(0, i)};
            }
        }
    }

    return {next_path: "", component: path};
};


exports.SplitPathComponents = function(path) {
    var components = [];

    while (path != "") {
        var item = exports.ConsumeComponent(path);
        if (item.component != "") {
            components.push(item.component);
        }
        path = item.next_path;
    }

    return components;
};

exports.Join = function(components) {
    var result = "";
    for (var i = 0; i < components.length; i++) {
        result = exports.PathJoin(result, components[i]);
    }

    return result;
};

// A safe version of nested dict dereference. If the member is not
// found, just returns a null instead of raising an exception.
exports.Get = function(item, member) {
    var components = member.split('.');

    for(var i=0; i<components.length; i++) {
        if (angular.isObject(item)) {
            item = item[components[i]];
        }
    }
    return item;
};
