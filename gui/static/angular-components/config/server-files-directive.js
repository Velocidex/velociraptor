'use strict';

goog.module('grrUi.config.serverFilesDirective');
goog.module.declareLegacyNamespace();


const ServerFilesController = function(
    $scope, grrRoutingService) {
  /** @private {!angular.Scope} */
  this.scope_ = $scope;

  /** @private {!grrUi.routing.routingService.RoutingService} */
  this.grrRoutingService_ = grrRoutingService;

  /** @type {string} */
  this.selectedDirPath;

  /** @type {string} */
  this.viewMode = 'list';

  /** @type {string} */
  this.tab = 'stats';

  /** @type {number|undefined} */
  this.fileVersion;

  this.grrRoutingService_.uiOnParamsChanged(
      this.scope_, ['path', 'version', 'mode', 'tab'],
      this.onUrlRoutingParamsChanged_.bind(this));

  this.scope_.$watchGroup(['controller.selectedDirPath',
                           'controller.fileVersion',
                           'controller.viewMode',
                           'controller.tab'],
                          this.onFileContextRoutingParamsChange_.bind(this));
};



/**
 * Handles changes of the routing-related params in the URL. Updates
 * file context.
 *
 * @param {!Array<string>} params Changed routing params.
 * @private
 */
ServerFilesController.prototype.onUrlRoutingParamsChanged_ = function(params) {
  this.selectedDirPath = params[0];
  this.fileVersion = parseInt(params[1], 10) || undefined;
  this.viewMode = params[2] || 'list';
  this.tab = params[3] || 'stats';
};

ServerFilesController.prototype.onFileContextRoutingParamsChange_ = function() {
  var params = {
    path: this.selectedDirPath,
  };
  if (!this.viewMode || this.viewMode == 'list') {
    params['mode'] = undefined;
  } else {
    params['mode'] = this.viewMode;
  }

  if (!this.tab || this.tab == 'stats') {
    params['tab'] = undefined;
  } else {
    params['tab'] = this.tab;
  }

  this.grrRoutingService_.go('server_files', params);
};


/**
 * FileViewDirective definition.
 * @return {angular.Directive} Directive definition object.
 */
exports.ServerFilesDirective = function() {
  return {
    restrict: 'E',
    scope: {},
    templateUrl: '/static/angular-components/config/server-files.html',
    controller: ServerFilesController,
    controllerAs: 'controller'
  };
};


/**
 * Name of the directive in Angular.
 *
 * @const
 * @export
 */
exports.ServerFilesDirective.directive_name = 'grrServerFiles';
