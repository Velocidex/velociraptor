'use strict';

goog.module('grrUi.artifact.toolViewerDirective');
goog.module.declareLegacyNamespace();

var ERROR_EVENT_NAME = 'ServerError';

const ToolViewerController = function($scope, $rootScope, $uibModal, grrApiService) {
    this.scope_ = $scope;
    this.rootScope_ = $rootScope;

    this.uibModal_ = $uibModal;
    this.grrApiService_ = grrApiService;

    this.tool;
    this.updateToolInfo();

    this.inflight = false;
    this.csrf = window.CsrfToken;
};

ToolViewerController.prototype.updateToolInfo = function() {
    var url = 'v1/GetToolInfo';
    var params = {name: this.scope_['name']};
    var self = this;

    this.grrApiService_.get(url, params).then(function(response) {
        self.tool = response.data;
    });
};

ToolViewerController.prototype.setToolInfo = function(tool) {
    var url = 'v1/SetToolInfo';
    var params = tool;
    var self = this;

    self.inflight = true;
    this.grrApiService_.post(url, params).then(function(response) {
        self.tool = response.data;
        self.inflight = false;
    }, function() {
        self.inflight = false;
    });
};

ToolViewerController.prototype.toolDialog = function(e) {
    var self = this;
    var modalScope = this.scope_.$new();
    modalScope["name"] = this.scope_["name"];
    modalScope["controller"] = this;

    modalScope.uploadFile = function(e) {
        var files = {file: self.tool_file};
        var params = self.tool;

        self.inflight = true;
        self.grrApiService_.upload(
            "v1/UploadTool", files, params).then(function(response) {
                self.tool = response.data;
                self.inflight = false;
            }, function(error) {
                self.inflight = false;
                self.rootScope_.$broadcast(
                    ERROR_EVENT_NAME, {message: error.data});
            });
        e.preventDefault();
        e.stopPropagation();
        return false;
    };

    modalScope.file_changed = function(element) {
        self.tool_file = element.files[0];
    };

    modalScope.serve_upstream = function() {
        let tool = Object.assign({}, self.tool);
        tool.serve_url = "";
        tool.serve_locally = false;
        self.setToolInfo(tool);
    };

    modalScope.serve_locally = function() {
        let tool = Object.assign({}, self.tool);
        tool.serve_locally = true;
        tool.materialize = true;
        self.setToolInfo(tool);
    };

    modalScope.calculateHash = function() {
        let tool = Object.assign({}, self.tool);
        tool.materialize = true;
        self.setToolInfo(tool);
    };

    modalScope.redownloadFile = function() {
        let tool = Object.assign({}, self.tool);
        tool.hash = "";
        tool.filename = "";
        tool.materialize = true;
        self.setToolInfo(tool);
    };

    modalScope.reject = function() {
        modalInstance.close();
        self.updateToolInfo();
    };

    var modalInstance = self.uibModal_.open({
        templateUrl: window.base_path+'/static/angular-components/artifact/tool-viewer-dialog.html',
        scope: modalScope,
        windowClass: 'wide-modal high-modal',
        size: "lg",
    });

    e.preventDefault();
    e.stopPropagation();
    return false;
};

exports.ToolViewerDirective = function() {
  return {
      scope: {
          name: "@",
      },
      restrict: 'E',
      templateUrl: window.base_path+'/static/angular-components/artifact/tool-viewer.html',
      controller: ToolViewerController,
      controllerAs: 'controller'
  };
};

exports.ToolViewerDirective.directive_name = 'grrToolViewer';
