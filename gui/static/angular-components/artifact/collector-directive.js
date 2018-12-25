'use strict';

goog.module('grrUi.artifact.collectorDirective');
goog.module.declareLegacyNamespace();


const ArtifactCollectorController = function(
    $scope, grrRoutingService) {
  this.scope_ = $scope;

    this.clientId;

    /** @private {!grrUi.routing.routingService.RoutingService} */
    this.grrRoutingService_ = grrRoutingService;
    this.grrRoutingService_.uiOnParamsChanged(this.scope_, 'clientId',
                                              this.onClientIdChange_.bind(this));

    this.descriptor = {
        name: "ArtifactCollector",
        args_type:"ArtifactCollectorArgs",
        default_args:{
            "@type": "type.googleapis.com/proto.ArtifactCollectorArgs"
        }
    };
};

ArtifactCollectorController.prototype.onClientIdChange_ = function(clientId) {
  this.clientId = clientId;
};


exports.ArtifactCollectorDirective = function() {
  return {
      scope: {
          'clientId': '=',
      },
      restrict: 'E',
      templateUrl: '/static/angular-components/artifact/collector.html',
      controller: ArtifactCollectorController,
      controllerAs: 'controller'
  };
};

/**
 * Directive's name in Angular.
 *
 * @const
 * @export
 */
exports.ArtifactCollectorDirective.directive_name = 'grrArtifactCollector';
