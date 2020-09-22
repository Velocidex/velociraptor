'use strict';

goog.module('grrUi.client.virtualFileSystem.fileDetailsDirective');

const {REFRESH_FILE_EVENT} = goog.require('grrUi.client.virtualFileSystem.events');

/**
 * Controller for FileDetailsDirective.
 *
 * @constructor
 * @param {!angular.Scope} $scope
 * @param {!grrUi.core.apiService.ApiService} grrApiService
 * @ngInject
 */
const FileDetailsController = function(
  $rootScope, $scope, grrApiService) {
  /** @private {!angular.Scope} */
  this.scope_ = $scope;

  this.rootScope_ = $rootScope;

  /** @private {!grrUi.core.apiService.ApiService} */
  this.grrApiService_ = grrApiService;

  /** @type {!grrUi.client.virtualFileSystem.fileContextDirective.FileContextController} */
  this.fileContext;

  /** @type {string} */
  this.currentTab = 'stats';

  this.fileIsDirectory;

  /** @type {?object} */
  this.params;

  this.reporting_params = {};

  this.rootScope_.$on(REFRESH_FILE_EVENT,
                      this.onFilePathChange_.bind(this));

  this.scope_.$watch('controller.fileContext.selectedFilePath',
                     this.onFilePathChange_.bind(this));
  this.scope_.$watch('currentTab', this.onDirectiveTabChange_.bind(this));
  this.scope_.$watch('controller.currentTab', this.onControllerTabChange_.bind(this));
};



/**
 * Handles currentTab scope attribute changes.
 *
 * @param {string} newValue
 * @private
 */
FileDetailsController.prototype.onDirectiveTabChange_ = function(newValue) {
  if (angular.isString(newValue)) {
    this.currentTab = newValue;
  }
};

FileDetailsController.prototype.onFilePathChange_ = function(newValue) {
    this.params = null;

    if (angular.isUndefined(this.fileContext.selectedRow)) {
        return;
    }


    var download = this.fileContext.selectedRow.Download;
    this.fileHasContent = angular.isObject(download);

    var filePath = this.fileContext.selectedFilePath;
    this.params = {
        path: '/clients/' + this.fileContext.clientId + '/' + filePath,
        client_id: this.fileContext.clientId,
    };

  this.fileIsDirectory = this.fileContext.selectedRow.Mode[0] == "d";
  this.fileIsCSV = this.fileContext.selectedRow.Name.endsWith(".csv");

    this.reportingParameters();
};

/**
 * Handles controller's currentTab attribute changes.
 *
 * @param {string} newValue
 * @param {string} oldValue
 * @private
 */
FileDetailsController.prototype.onControllerTabChange_ = function(newValue, oldValue) {
  if (newValue !== oldValue) {
    this.scope_['currentTab'] = newValue;
  }
};


FileDetailsController.prototype.parseFilenameToTimestamp = function(filename) {
  return Date.parse(filename.replace(/\.csv$/, "") + " 00:00:00 GMT") / 1000;
};

FileDetailsController.prototype.reportingParameters = function() {
    this.reporting_params = null;

    if (!angular.isString(this.params.path)) {
        return;
    }
    var components = this.params.path.split('/');
    if (components.length != 4) {
        return;
    }

    if (components[1] == "artifacts") {
        var artifact_name = components[2];
        var flowId = components[3];

        // Strip possible extensions.
        flowId = flowId.replace(/\.csv$/, "");

        this.reporting_params = {
            "artifact": artifact_name,
            "client_id": this.params["client_id"],
            "flowId": flowId,
            "type": "CLIENT",
        };
    }

};

/**
 * FileDetailsDirective definition.
 *
 * @return {angular.Directive} Directive definition object.
 */
exports.FileDetailsDirective = function() {
  return {
    restrict: 'E',
    scope: {
      currentTab: '='
    },
    require: '^grrFileContext',
    templateUrl: window.base_path+'/static/angular-components/client/virtual-file-system/file-details.html',
    controller: FileDetailsController,
    controllerAs: 'controller',
    link: function(scope, element, attrs, fileContextController) {
      scope.controller.fileContext = fileContextController;
    }
  };
};


/**
 * Name of the directive in Angular.
 *
 * @const
 * @export
 */
exports.FileDetailsDirective.directive_name = 'grrFileDetails';
