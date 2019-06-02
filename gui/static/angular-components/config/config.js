'use strict';

goog.module('grrUi.config.config');
goog.module.declareLegacyNamespace();

const {coreModule} = goog.require('grrUi.core.core');



/**
 * Angular module for config-related UI.
 */
exports.configModule = angular.module('grrUi.config', [coreModule.name]);
