'use strict';

goog.module('grrUi.utils.utils');
goog.module.declareLegacyNamespace();

const {TimestampDirective} = goog.require('grrUi.utils.timestampDirective');
const {TimestampSecondsDirective} = goog.require('grrUi.utils.timestampSecondsDirective');
const {coreModule} = goog.require('grrUi.core.core');
const {routingModule} = goog.require('grrUi.routing.routing');
const {VQLDirective} = goog.require('grrUi.utils.vqlDirective');
const {ClientLabelFormDirective} = goog.require('grrUi.utils.clientLabelFormDirective');

exports.utilsModule = angular.module('grrUi.utils', [
  coreModule.name, routingModule.name, 'ui.bootstrap'
]);

exports.utilsModule.directive(
    TimestampDirective.directive_name, TimestampDirective);
exports.utilsModule.directive(
    TimestampSecondsDirective.directive_name, TimestampSecondsDirective);
exports.utilsModule.directive(
  VQLDirective.directive_name, VQLDirective);
exports.utilsModule.directive(
  ClientLabelFormDirective.directive_name, ClientLabelFormDirective);
