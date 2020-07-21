'use strict';

goog.module('grrUi.client.client');
goog.module.declareLegacyNamespace();

const {AddClientsLabelsDialogDirective} = goog.require('grrUi.client.addClientsLabelsDialogDirective');
const {BuildCollectorDirective} = goog.require('grrUi.client.buildCollectorDirective');
const {ClientContextDirective} = goog.require('grrUi.client.clientContextDirective');
const {ClientDialogService} = goog.require('grrUi.client.clientDialogService');
const {ClientsListDirective} = goog.require('grrUi.client.clientsListDirective');
const {HostInfoDirective} = goog.require('grrUi.client.hostInfoDirective');
const {ShellViewerDirective} = goog.require('grrUi.client.shellViewerDirective');
const {ShellCellDirective} = goog.require('grrUi.client.shellCellDirective');
const {RemoveClientsLabelsDialogDirective} = goog.require('grrUi.client.removeClientsLabelsDialogDirective');
const {coreModule} = goog.require('grrUi.core.core');
const {virtualFileSystemModule} = goog.require('grrUi.client.virtualFileSystem.virtualFileSystem');


/**
 * Angular module for clients-related UI.
 */
exports.clientModule = angular.module('grrUi.client', [
    virtualFileSystemModule.name, coreModule.name, 'ui.select',
]);

exports.clientModule.directive(
    AddClientsLabelsDialogDirective.directive_name,
    AddClientsLabelsDialogDirective);
exports.clientModule.directive(
    BuildCollectorDirective.directive_name, BuildCollectorDirective);
exports.clientModule.directive(
    ClientContextDirective.directive_name, ClientContextDirective);
exports.clientModule.directive(
    ClientsListDirective.directive_name, ClientsListDirective);
exports.clientModule.directive(
    HostInfoDirective.directive_name, HostInfoDirective);
exports.clientModule.directive(
    ShellViewerDirective.directive_name, ShellViewerDirective);
exports.clientModule.directive(
    ShellCellDirective.directive_name, ShellCellDirective);
exports.clientModule.directive(
    RemoveClientsLabelsDialogDirective.directive_name,
    RemoveClientsLabelsDialogDirective);
exports.clientModule.service(
    ClientDialogService.service_name, ClientDialogService);
