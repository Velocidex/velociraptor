'use strict';

goog.module('grrUi.artifact.collectorDirective');
goog.module.declareLegacyNamespace();


/**
 * Controller for ArtifactCollectorController
 *
 * @param {!angular.Scope} $scope
 * @param {!grrUi.routing.routingService.RoutingService} grrRoutingService
 * @constructor
 * @ngInject
 */
const ArtifactCollectorController = function(
    $scope, grrRoutingService) {
    /** @private {!angular.Scope} */
    this.scope_ = $scope;

    /** @export {?string} */
    this.clientId;

    /** @private {!grrUi.routing.routingService.RoutingService} */
    this.grrRoutingService_ = grrRoutingService;
    grrRoutingService.uiOnParamsChanged(this.scope_, 'clientId',
                                        this.onClientIdChange_.bind(this));

    /** @export {?object} */
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
