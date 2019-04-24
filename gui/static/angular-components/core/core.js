'use strict';

goog.module('grrUi.core.core');
goog.module.declareLegacyNamespace();


const {ApiItemsProviderDirective} = goog.require('grrUi.core.apiItemsProviderDirective');
const {ApiService} = goog.require('grrUi.core.apiService');
const {ClockDirective} = goog.require('grrUi.core.clockDirective');
const {CsvViewerDirective} = goog.require('grrUi.core.csvViewerDirective');
const {ConfirmationDialogDirective} = goog.require('grrUi.core.confirmationDialogDirective');
const {DialogService} = goog.require('grrUi.core.dialogService');
const {DisableIfNoTraitDirective} = goog.require('grrUi.core.disableIfNoTraitDirective');
const {ForceRefreshDirective} = goog.require('grrUi.core.forceRefreshDirective');
const {GlobalNotificationsDirective} = goog.require('grrUi.core.globalNotificationsDirective');
const {HexNumberFilter} = goog.require('grrUi.core.hexNumberFilter');
const {InfiniteTableDirective} = goog.require('grrUi.core.infiniteTableDirective');
const {LoadingIndicatorService} = goog.require('grrUi.core.loadingIndicatorService');
const {PagedFilteredTableDirective, TableBottomDirective, TableTopDirective} = goog.require('grrUi.core.pagedFilteredTableDirective');
const {PeriodicRefreshDirective} = goog.require('grrUi.core.periodicRefreshDirective');
const {ReflectionService} = goog.require('grrUi.core.reflectionService');
const {SearchBoxDirective} = goog.require('grrUi.core.searchBoxDirective');
const {ErrorLabelDirective} = goog.require('grrUi.core.errorLabelDirective');
const {ServerErrorButtonDirective} = goog.require('grrUi.core.serverErrorButtonDirective');
const {ServerErrorDialogDirective} = goog.require('grrUi.core.serverErrorDialogDirective');
const {ServerErrorInterceptorFactory} = goog.require('grrUi.core.serverErrorInterceptorFactory');
const {LoadingIndicatorDirective} = goog.require('grrUi.core.loadingIndicatorDirective');
const {ServerErrorPreviewDirective} = goog.require('grrUi.core.serverErrorPreviewDirective');
const {SplitterDirective, SplitterPaneDirective} = goog.require('grrUi.core.splitterDirective');
const {TimeService} = goog.require('grrUi.core.timeService');
const {TimeSinceFilter} = goog.require('grrUi.core.timeSinceFilter');
const {TimestampFilter} = goog.require('grrUi.core.timestampFilter');
const {WizardFormDirective} = goog.require('grrUi.core.wizardFormDirective');
const {WizardFormPageDirective} = goog.require('grrUi.core.wizardFormPageDirective');



/**
 * Angular module for core GRR UI components.
 */
exports.coreModule =
    angular.module('grrUi.core', ['ngCookies', 'ui.bootstrap', 'datatables']);


exports.coreModule.directive(
    ApiItemsProviderDirective.directive_name, ApiItemsProviderDirective);
exports.coreModule.directive(
    ForceRefreshDirective.directive_name, ForceRefreshDirective);
exports.coreModule.directive(
    PagedFilteredTableDirective.directive_name, PagedFilteredTableDirective);
exports.coreModule.directive(
    TableTopDirective.directive_name, TableTopDirective);
exports.coreModule.directive(
    TableBottomDirective.directive_name, TableBottomDirective);
exports.coreModule.directive(
    PeriodicRefreshDirective.directive_name, PeriodicRefreshDirective);
exports.coreModule.directive(
    InfiniteTableDirective.directive_name, InfiniteTableDirective);
exports.coreModule.directive(
    SplitterDirective.directive_name, SplitterDirective);
exports.coreModule.directive(
    SplitterPaneDirective.directive_name, SplitterPaneDirective);
exports.coreModule.directive(ClockDirective.directive_name, ClockDirective);
exports.coreModule.directive(
    CsvViewerDirective.directive_name, CsvViewerDirective);
exports.coreModule.directive(
    WizardFormDirective.directive_name, WizardFormDirective);
exports.coreModule.directive(
    WizardFormPageDirective.directive_name, WizardFormPageDirective);
exports.coreModule.directive(
    ConfirmationDialogDirective.directive_name, ConfirmationDialogDirective);
exports.coreModule.directive(
    DisableIfNoTraitDirective.directive_name, DisableIfNoTraitDirective);
exports.coreModule.directive(
    SearchBoxDirective.directive_name, SearchBoxDirective);
exports.coreModule.directive(
    ErrorLabelDirective.directive_name, ErrorLabelDirective);
exports.coreModule.directive(
    ServerErrorButtonDirective.directive_name, ServerErrorButtonDirective);
exports.coreModule.directive(
    ServerErrorDialogDirective.directive_name, ServerErrorDialogDirective);
exports.coreModule.directive(
    ServerErrorPreviewDirective.directive_name, ServerErrorPreviewDirective);
exports.coreModule.directive(
    GlobalNotificationsDirective.directive_name, GlobalNotificationsDirective);
exports.coreModule.service(ApiService.service_name, ApiService);
exports.coreModule.service(ReflectionService.service_name, ReflectionService);
exports.coreModule.service(TimeService.service_name, TimeService);
exports.coreModule.service(DialogService.service_name, DialogService);
exports.coreModule.directive(
    LoadingIndicatorDirective.directive_name, LoadingIndicatorDirective);
exports.coreModule.service(
    LoadingIndicatorService.service_name, LoadingIndicatorService);
exports.coreModule.filter(HexNumberFilter.filter_name, HexNumberFilter);
exports.coreModule.filter(TimeSinceFilter.filter_name, TimeSinceFilter);
exports.coreModule.filter(TimestampFilter.filter_name, TimestampFilter);
exports.coreModule.factory(
    ServerErrorInterceptorFactory.factory_name, ServerErrorInterceptorFactory);

exports.coreModule.config(function($httpProvider) {
  $httpProvider.interceptors.push(ServerErrorInterceptorFactory.factory_name);
});
