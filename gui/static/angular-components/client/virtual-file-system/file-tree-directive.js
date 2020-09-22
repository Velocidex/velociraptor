'use strict';

goog.module('grrUi.client.virtualFileSystem.fileTreeDirective');
goog.module.declareLegacyNamespace();

const {REFRESH_FOLDER_EVENT} = goog.require('grrUi.client.virtualFileSystem.events');
const {ensurePathIsFolder, getFolderFromPath} = goog.require('grrUi.client.virtualFileSystem.utils');
const {getFileId} = goog.require('grrUi.client.virtualFileSystem.fileViewDirective');
const {PathJoin, ConsumeComponent} = goog.require('grrUi.core.utils');

/**
 * Controller for FileTreeDirective.
 *
 * @constructor
 * @param {!angular.Scope} $rootScope
 * @param {!angular.Scope} $scope
 * @param {!angular.jQuery} $element
 * @param {!grrUi.core.apiService.ApiService} grrApiService
 * @param {!grrUi.routing.routingService.RoutingService} grrRoutingService
 * @ngInject
 */
const FileTreeController = function(
    $rootScope, $scope, $element, grrApiService, grrRoutingService) {
  /** @private {!angular.Scope} */
  this.rootScope_ = $rootScope;

  /** @private {!angular.Scope} */
  this.scope_ = $scope;

  /** @private {!angular.jQuery} */
  this.element_ = $element;

  /** @private {!Object} */
  this.treeElement_ = $element.find('#file-tree');

  /** @private {!grrUi.core.apiService.ApiService} */
  this.grrApiService_ = grrApiService;

  /** @type {!grrUi.client.virtualFileSystem.fileContextDirective.FileContextController} */
  this.fileContext;


    this.rootScope_.$on(REFRESH_FOLDER_EVENT,
                        this.onRefreshFolderEvent_.bind(this));

  this.scope_.$watch('controller.fileContext.clientId',
      this.onClientIdChange_.bind(this));
  this.scope_.$watch('controller.fileContext.selectedDirPath',
      this.onSelectedFilePathChange_.bind(this));
};



/**
 * Handles changes of clientId binding.
 *
 * @private
 */
FileTreeController.prototype.onClientIdChange_ = function() {
  if (angular.isDefined(this.fileContext['clientId'])) {
    this.initTree_();
  }
};

/**
 * Initializes the jsTree instance.
 *
 * @private
 */
FileTreeController.prototype.initTree_ = function() {
  var self = this;
  this.treeElement_.jstree({
    'core' : {
        'multiple': false,
        'themes': {
            'name': 'proton',
            'responsive': true
        },
        'data' : function (node, cb) {
            if (node.id === '#') {
                self.getChildFiles_('/', node).then(cb);

            } else {
                self.getChildFiles_(node.data.path).then(cb);
            }
        }
    }
  });

  this.treeElement_.on('changed.jstree', function (e, data) {
    // We're only interested in actual "select" events (not "ready" event,
    // which is sent when the node is loaded).
    if (data['action'] !== 'select_node') {
      return;
    }
    var selectionId = data.selected[0];
    var node = this.treeElement_.jstree('get_node', selectionId);
    var folderPath =  node.data.path;

    if (getFolderFromPath(this.fileContext['selectedDirPath']) === folderPath) {
        this.rootScope_.$broadcast(REFRESH_FOLDER_EVENT,
                                   ensurePathIsFolder(folderPath));
    } else {
        this.fileContext.selectFile(ensurePathIsFolder(folderPath));
    }

      // This is needed so that when user clicks on an already opened node,
      // it gets refreshed.
      var treeInstance = data['instance'];
      treeInstance['refresh_node'](data.node);
  }.bind(this));

  this.treeElement_.on('close_node.jstree', function(e, data) {
    data.node['data']['refreshOnOpen'] = true;
  }.bind(this));

  this.treeElement_.on('open_node.jstree', function(e, data) {
    if (data.node['data']['refreshOnOpen']) {
      data.node['data']['refreshOnOpen'] = false;

      var treeInstance = data['instance'];
      treeInstance['refresh_node'](data.node);
    }
  }.bind(this));

  this.treeElement_.on("loaded.jstree", function () {
    var selectedDirPath = this.fileContext['selectedDirPath'];
    if (selectedDirPath) {
      this.expandToFilePath_(getFileId(getFolderFromPath(selectedDirPath)),
                             true);
    }
  }.bind(this));

  // Selecting a node automatically opens it
  this.treeElement_.on('select_node.jstree', function(event, data) {
    $(this)['jstree']('open_node', '#' + data.node.id);
    return true;
  });
};

/**
 * Retrieves the child directories for the current folder.
 * @param {string} folderPath The path of the current folder.
 * @return {angular.$q.Promise} A promise returning the child files when resolved.
 * @private
 */
FileTreeController.prototype.getChildFiles_ = function(folderPath) {
  var clientId_ = this.fileContext['clientId'];
  var url = 'v1/VFSListDirectory/' + clientId_;
  var params = { 'vfs_path': folderPath || '/' };

  return this.grrApiService_.get(url, params).then(
    function(response) {
      return this.parseFileResponse_(response, folderPath);
    }.bind(this), function(response) {
      this.fileContext.selectedDirPathData = undefined;
      this.fileContext.selectedRow = undefined;
    }.bind(this));
};

/**
 * Parses the API response and converts it to the structure jsTree requires.
 * @param {Object} response The server response.
 * @return {Array} A list of files in a jsTree-compatible structure.
 * @private
 */
FileTreeController.prototype.parseFileResponse_ = function(response, folderPath) {
  if (angular.isUndefined(response.data.Response)) {
    this.fileContext.selectedDirPathData = undefined;
    this.fileContext.selectedRow = undefined;
    return [];
  }

    // Only update the file context if this is the node it is
    // watching. jstree will actually refresh many nodes all the time
    // but we only want to export the data about the selected one back
    // to the context.
    if (angular.isString(this.fileContext.selectedDirPath) &&
        ensurePathIsFolder(this.fileContext.selectedDirPath) == ensurePathIsFolder(folderPath)) {
        this.fileContext.selectedDirPathData = response.data;
    };

    var files = JSON.parse(response.data.Response);
    var result = [];
    angular.forEach(files, function(file) {
        var mode = file["Mode"][0];
        if (mode == "d" || mode == "L") {
            var filePath = file['Name'];
            var fullFilePath = PathJoin(folderPath, filePath);
            var fileId = getFileId(fullFilePath);
            result.push({
                id: fileId,
                text: file['Name'],
                data: {
                    name: file['Name'],
                    path: fullFilePath,
                },
                children: true  // always set to true to show the triangle
            });
        }
    }.bind(this));

    return result;
};

/**
 * Is triggered by REFRESH_FOLDER_EVENT.
 * @private
 */
FileTreeController.prototype.onRefreshFolderEvent_ = function(e, path) {
  if (angular.isUndefined(path)) {
    path = this.fileContext['selectedDirPath'];
  }

    var nodeId = getFileId(getFolderFromPath(path));
    var node = $('#' + nodeId);
    var tree_node = this.treeElement_.jstree(true);
    if (angular.isFunction(tree_node['refresh_node'])) {
        tree_node['refresh_node'](node);
    }
};

/**
 * Is triggered whenever the selected folder path changes
 * @private
 */
FileTreeController.prototype.onSelectedFilePathChange_ = function() {
  var selectedDirPath = this.fileContext['selectedDirPath'];

  if (selectedDirPath) {
    var selectedFolderPath = getFolderFromPath(selectedDirPath);
    this.expandToFilePath_(getFileId(selectedFolderPath), true);
  }
};

/**
 * Selects a folder defined by the given path. If the path is not available, it
 * selects the closest parent folder.
 *
 * @param {string} filePathId The id of the folder to select.
 * @param {boolean=} opt_suppressEvent If true, no 'jstree.changed' event will
 *     be sent when the node is selected.
 * @private
 */
FileTreeController.prototype.expandToFilePath_ = function(
    filePathId, opt_suppressEvent) {
  if (!filePathId) {
    return;
  }
  var element = this.treeElement_;
  var parts = filePathId.split('-');

  var cb = function(i, prev_node) {
    var id_to_open = parts.slice(0, i + 1).join('-');
    var node = $('#' + id_to_open);

    if (node.length) {
      if (parts[i + 1]) {
        // There are more nodes to go, proceed recursively.
        element.jstree('open_node', node, function() { cb(i + 1, node); },
                       'no_hash');
      } else {
        // Target node: select it.
        element.jstree(true)['deselect_all'](true);
        element.jstree(true)['select_node'](node, opt_suppressEvent);
      }
    } else if (prev_node) {
      // Node can't be found, finish by selecting last available parent.
      element.jstree(true)['deselect_all'](true);
      element.jstree(true)['select_node'](prev_node, opt_suppressEvent);
    }
  }.bind(this);

  cb(0, null);
};


/**
 * FileTreeDirective definition.
 * @return {angular.Directive} Directive definition object.
 */
exports.FileTreeDirective = function() {
  return {
    restrict: 'E',
    scope: {},
    require: '^grrFileContext',
    templateUrl: window.base_path+'/static/angular-components/client/virtual-file-system/file-tree.html',
    controller: FileTreeController,
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
exports.FileTreeDirective.directive_name = 'grrFileTree';
