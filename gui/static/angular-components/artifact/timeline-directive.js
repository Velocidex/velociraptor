'use strict';

goog.module('grrUi.artifact.timelineDirective');

/**
 * Controller for TimelineDirective.
 *
 * @constructor
 * @param {!angular.Scope} $scope
 * @ngInject
 */
const TimelineController = function($scope) {
    /** @private {!angular.Scope} */
    this.scope_ = $scope;

    /** @type {object} */
    this.params;

    /** @type {?string} */
    this.pageData;
};

/**
 * TimelineDirective definition.
 * @return {angular.Directive} Directive definition object.
 */
exports.TimelineDirective = function() {
    return {
        scope: {
            params: '=',
            value: '=',
        },
        restrict: 'E',
        link:  function(scope, elem, attrs){
            var params = scope.params || {};
            var value = scope.value;
            var columns = value.Columns;
            var timestamp_column = value.Columns[0];
            var name_column = value.Columns[1];
            var data = [];

            elem = $(elem);
            var new_element = $("<div>").appendTo(elem).show();
            var container = new_element[0];
            var rows = JSON.parse(value.Response);
            for (var i=0; i<rows.length; i++) {
                if (i > 1000) {  // Protect ourselves from overuse.
                    break;
                }
                var timestamp = rows[i][timestamp_column];
                var title = rows[i][name_column];
                data.push({id: i, content: title, start: timestamp});
            }

            var options = {
                stack: true,
                maxHeight: 400,
                minHeight: 400,
                editable: true,
                margin: {
                    item: 10, // minimal margin between items
                    axis: 5   // minimal margin between items and the axis
                },
                orientation: 'top',
                clickToUse: true
            };

            var dataset = new vis.DataSet(data);
            var timeline = new vis.Timeline(container, dataset, options);
        },
        controller: TimelineController,
        controllerAs: 'controller',
    };
};


/**
 * Name of the directive in Angular.
 *
 * @const
 * @export
 */
exports.TimelineDirective.directive_name = 'grrTimeline';
