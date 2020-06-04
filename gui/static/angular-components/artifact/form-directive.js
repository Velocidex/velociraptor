'use strict';

goog.module('grrUi.artifact.formDirective');

const FormController = function($scope) {
  /** @private {!angular.Scope} */
  this.scope_ = $scope;
  this.popup_opened = false;
    this.date_time;
    this.text_size = 1;
    this.dateOptions = {allowInvalid: true};
    this.altInputFormats = ["M!/d!/yyyy"];

    if (this.scope_["type"] == "timestamp") {
        var value = this.scope_.value[this.scope_.field];
        if (angular.isNumber(value)) {
            this.date_time = new Date(value * 1000);
        }
    }
  $scope.$watch('controller.date_time',
                this.onDateChange_.bind(this));
};


FormController.prototype.onDateChange_ = function() {
    if (angular.isDefined(this.date_time)) {
        try {
            this.scope_.value[this.scope_.field] = (
                this.date_time.getTime() / 1000).toString();
        } catch(e) {
            this.scope_.value[this.scope_.field] = this.date_time;
        }
    }
};

FormController.prototype.openDatePopup = function(e) {
    this.popup_opened = true;
    if (angular.isDefined(e)) {
        e.stopPropagation();
        e.preventDefault();
        return false;
    }
};

FormController.prototype.resizeTextArea = function(e) {
    if (this.text_size == 1) {
        this.text_size = 10;
    } else {
        this.text_size = 1;
    };
    e.stopPropagation();
    return false;
};

exports.FormDirective = function() {
  return {
    restrict: 'E',
    scope: {
        field: '=',
        value: '=',
        info: '=',
        type: '=',
        description: '=',
    },
    templateUrl: '/static/angular-components/artifact/form.html',
    controller: FormController,
    controllerAs: 'controller'
  };
};


exports.FormDirective.directive_name = 'grrForm';
