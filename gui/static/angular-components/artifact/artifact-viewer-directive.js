'use strict';

goog.module('grrUi.artifact.artifactViewerDirective');


const ArtifactViewerController = function(
    $scope, grrApiService, $uibModal) {
    this.scope_ = $scope;
    this.grrApiService_ = grrApiService;
    this.uibModal_ = $uibModal;

    /** @export {Object<string, Object>} */
    this.descriptors = {};

    /** @export {string} */
    this.descriptorsError;

    /** @export {Object} */
    this.selectedName;
    this.isCustom = false;

    this.reportParams = {};

    // A list of descriptors that matched the search term.
    this.matchingDescriptors = [];
    this.scope_.$watch('controller.search',
                       this.onSearchChange_.bind(this));

    this.uiTraits = {};
    this.grrApiService_.getCached('v1/GetUserUITraits').then(function(response) {
        this.uiTraits = response.data['interface_traits'];
    }.bind(this), function(error) {
        if (error['status'] == 403) {
            this.error = 'Authentication Error';
        } else {
            this.error = error['statusText'] || ('Error');
        }
    }.bind(this));
};

ArtifactViewerController.prototype.onSearchChange_ = function() {
    var self = this;
    this.grrApiService_.get(
        "/v1/GetArtifacts", {
            search_term: self.search,
        }).then(
            function(response){
                self.matchingDescriptors = [];

                for(var i=0; i<response.data.items.length; i++) {
                    var desc = response.data.items[i];
                    self.descriptors[desc.name] = desc;
                    self.matchingDescriptors.push(desc);
                };
            }, function(err) {
                self.descriptorsError = err;
            });
};

ArtifactViewerController.prototype.selectName = function(name, e) {
    this.selectedName = name;
    this.isCustom = name.startsWith("Custom.");

    this.reportParams= {
        artifact: this.selectedName,
        type: "ARTIFACT_DESCRIPTION",
    };

    e.preventDefault();
    e.stopPropagation();
    return false;
};

ArtifactViewerController.prototype.updateArtifactDefinitions = function(name) {
    var url = 'v1/GetArtifactFile';
    var params = {
        name: name,
    };
    var self = this;

    this.error = "";
    var modalScope = this.scope_.$new();

    this.grrApiService_.get(url, params).then(function(response) {
        modalScope["artifact"] = response['data']['artifact'];
        modalScope.resolve = function() {
            modalInstance.close();

            // Update the search results.
            self.onSearchChange_();
        };

        var modalInstance = self.uibModal_.open({
            template: '<grr-add-artifact artifact="artifact" on-resolve="resolve()" />',
            scope: modalScope,
            windowClass: 'wide-modal high-modal',
            size: "lg",
        });
    });

    return false;
};


ArtifactViewerController.prototype.deleteArtifactDefinitions = function() {
  var self = this;
  self.modalInstance = self.uibModal_.open({
    templateUrl: '/static/angular-components/artifact/del_artifact.html',
    scope: self.scope_,
    size: "lg",
  });
};

ArtifactViewerController.prototype.deleteArtifactDefinitionsForReal = function() {
    var url = 'v1/SetArtifactFile';
    var params = {
      artifact: "name: " + this.selectedName,
      op: "DELETE",
    };

    this.grrApiService_.post(url, params).then(function(response) {
        if (response.data.error) {
            this.error = response.data['error_message'];
        } else {
          this.modalInstance.close();

          // Update the search results.
          this.onSearchChange_();
        }
    }.bind(this), function(error) {
        this.error = error;
    }.bind(this));
};



exports.ArtifactsViewerDirective = function() {
  return {
    restrict: 'E',
    scope: {},
    templateUrl: '/static/angular-components/artifact/' +
        'artifact-viewer.html',
    controller: ArtifactViewerController,
    controllerAs: 'controller'
  };
};


exports.ArtifactsViewerDirective.directive_name = 'grrArtifactViewer';
