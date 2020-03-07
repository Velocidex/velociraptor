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
    return path + "\"" + component.replace("\"", "\"\"") + "\"";
};



var component_quoted_regex = /^[\\/]?"((?:[^"\\]*(?:\\"?)?)+)"/;
var component_unquoted_regex = /^[\\/]?([^\\/]*)([\\/]?|$)/;

// Split an escaped path into components.
exports.SplitPathComponents = function(path) {
    var components = [];
    while(path.length > 0) {
        var match = path.match(component_quoted_regex);
        if (match != null && match.length >= 2) {
            components.push(match[1]);
            path = path.substr(match[0].length);
            continue;
        }

        match = path.match(component_unquoted_regex);
        if (match != null && match.length >= 2) {
            components.push(match[1]);
            path = path.substr(match[0].length);
            continue;
        }

        return path.split("/");
    }

    return components.filter(function (x) {return x.length > 0;});
};
