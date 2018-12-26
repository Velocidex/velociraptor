'use strict';

goog.module('grrUi.config.config');
goog.module.declareLegacyNamespace();

const {ServerFilesDirective} = goog.require('grrUi.config.serverFilesDirective');
const {coreModule} = goog.require('grrUi.core.core');



/**
 * Angular module for config-related UI.
 */
exports.configModule = angular.module('grrUi.config', [coreModule.name]);

exports.configModule.directive(
    ServerFilesDirective.directive_name, ServerFilesDirective);
