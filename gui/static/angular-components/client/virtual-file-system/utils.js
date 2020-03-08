'use strict';

goog.module('grrUi.client.virtualFileSystem.utils');
goog.module.declareLegacyNamespace();

const {SplitPathComponents, Join} = goog.require('grrUi.core.utils');


/**
 * Adds a trailing forward slash ('/') if it's not already present in the
 * path. In GRR VFS URLs trailing slash signifies directories.
 *
 * @param {string} path
 * @return {string} The given path with a single trailing slash added if needed.
 * @export
 */
exports.ensurePathIsFolder = function(path) {
  if (path.endsWith('/')) {
    return path;
  } else {
    return path + '/';
  }
};


/**
 * Strips last path component from the path. If the path ends with
 * a trailing slash, just the slash will be stripped.
 *
 * @param {?string} path
 * @return {string} The given path with a trailing slash stripped.
 * @export
 */
exports.getFolderFromPath = function(path) {
    if (!angular.isDefined(path)) {
        return;
    }

    if (path.endsWith('/')) {
        return path.replace(/\/+$/, '');
    };

    var components = SplitPathComponents(path);
    if (components.length == 0) {
        return "";
    }

    return Join(components.slice(0, components.length-1));
};
