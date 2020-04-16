'use strict';

goog.module('grrUi.sidebar.sidebar');
goog.module.declareLegacyNamespace();

const {ClientSummaryDirective} = goog.require('grrUi.sidebar.clientSummaryDirective');
const {NavLinkDirective} = goog.require('grrUi.sidebar.navLinkDirective');
const {NavigatorDirective} = goog.require('grrUi.sidebar.navigatorDirective');
const {coreModule} = goog.require('grrUi.core.core');

/**
 * Angular module for layout-related UI.
 */
exports.sidebarModule = angular.module('grrUi.sidebar', [coreModule.name]);

exports.sidebarModule.directive(
    ClientSummaryDirective.directive_name, ClientSummaryDirective);
exports.sidebarModule.directive(
    NavLinkDirective.directive_name, NavLinkDirective);
exports.sidebarModule.directive(
    NavigatorDirective.directive_name, NavigatorDirective);
