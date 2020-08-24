'use strict';

goog.module('grrUi.utils.timestampDirective');

/**
 * Controller for TimestampDirective.
 *
 * @param {!angular.Scope} $scope
 * @param {!angular.jQuery} $element
 * @param {!grrUi.core.timeService.TimeService} grrTimeService
 * @constructor
 * @ngInject
 */
const TimestampController = function(
    $scope, $element, grrTimeService) {
  /** @private {!angular.Scope} */
  this.scope_ = $scope;

  /** @type {?} */
  this.scope_.value;

  /** @private {?string} */
  this.formattedTimestamp;

  /** @private {Array<string>} */
  this.formattedTimestampComponents;

  /** @private {?number} */
  this.value;

  /** @private {!angular.jQuery} $element */
  this.element_ = $element;

  /** @private {grrUi.core.timeService.TimeService} grrTimeService */
  this.timeService_ = grrTimeService;

  this.scope_.$watch('::value', this.onValueChange.bind(this));
};



/**
 * Handles changes of scope.value attribute.
 *
 * @param {number} newValue Timestamp value in microseconds.
 * @suppress {missingProperties} as value can be anything.
 */
TimestampController.prototype.onValueChange = function(newValue) {
  if (angular.isDefined(newValue)) {
    if (newValue === null || newValue === 0) {
      this.formattedTimestamp = '-';
    } else {
      if (typeof newValue == 'string') {
        newValue = parseInt(newValue);
      }

      this.value = newValue / 1000;
      this.formattedTimestamp = this.timeService_.formatAsUTC(this.value);
      this.formattedTimestampComponents = this.formattedTimestamp.split(' ');
    }
  }
};

/**
 * Called when a user hovers the mouse over a timestamp to display the tooltip.
 */
TimestampController.prototype.onMouseEnter = function() {
  var span = $(this.element_).find('span')[0];

  if (angular.isDefined(this.value)) {
    span.title =
        this.timeService_.getFormattedDiffFromCurrentTime(Number(this.value));
  }
};

/**
 * Directive that displays RDFDatetime values.
 *
 * @return {!angular.Directive} Directive definition object.
 * @ngInject
 * @export
 */
exports.TimestampDirective = function() {
  return {
    scope: {
      value: '='
    },
    restrict: 'E',
    templateUrl: window.base_path+'/static/angular-components/utils/timestamp.html',
    controller: TimestampController,
    controllerAs: 'controller'
  };
};


/**
 * Directive's name in Angular.
 *
 * @const
 * @export
 */
exports.TimestampDirective.directive_name = 'grrTimestamp';
