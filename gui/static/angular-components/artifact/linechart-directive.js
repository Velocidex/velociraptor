'use strict';

goog.module('grrUi.artifact.lineChartDirective');

/**
 * Controller for LineChartDirective.
 *
 * @constructor
 * @param {!angular.Scope} $scope
 * @ngInject
 */
const LineChartController = function($scope) {
    /** @private {!angular.Scope} */
    this.scope_ = $scope;

    /** @type {object} */
    this.params;

    /** @type {?string} */
    this.pageData;
};

/**
 * LineChartDirective definition.
 * @return {angular.Directive} Directive definition object.
 */
exports.LineChartDirective = function() {
    var tooltip = $("<div class='flot-tooltip'></div>").css({
        position: "absolute",
        display: "none",
        border: "1px solid #fdd",
        padding: "2px",
        "background-color": "#fee",
        opacity: 0.80
    }).appendTo("body");

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
          var x_column = value.Columns[0];
          var series = [];
          var max_yaxis = 1;

          // Hide the tooltip when we change screen.
          scope.$on("$destroy", function() {
              tooltip.hide();
          });

          for (var c=1; c<columns.length;c++) {
              var yaxis = params[columns[c]+".yaxis"] || 1;
              if (yaxis > max_yaxis) {
                  max_yaxis = yaxis;
              }

              series.push({label: columns[c], data: [], yaxis: yaxis});
          }

          var rows = JSON.parse(value.Response);
          for (var i=0; i<rows.length; i++) {
              // The x axis is the first column.
              var x = rows[i][x_column];
              for (var c=1; c<value.Columns.length; c++) {
                  var y_column = value.Columns[c];
                  var y = rows[i][y_column];
                  series[c-1].data.push([x,y]);
              }
          }

          elem = $(elem);

          var plot = null;
          var placeholder = $("<div>").appendTo(elem);
          placeholder.bind("plotselected", function (event, ranges) {
              $.each(plot.getXAxes(), function(_, axis) {
                  var opts = axis.options;
                  opts.min = ranges.xaxis.from;
                  opts.max = ranges.xaxis.to;
              });
              plot.setupGrid();
              plot.draw();
              plot.clearSelection();
          });

          placeholder.bind("plotunselected", function (event) {
              $("#selection").text("");
          });

          placeholder.bind("plothover", function (event, pos, item) {
              if (item) {
                  var x = item.datapoint[0].toFixed(2),
                      y = item.datapoint[1].toFixed(2);

                  tooltip.html(item.series.label + " @ " + x + " = " + y)
                      .css({top: item.pageY+5, left: item.pageX+5})
                      .fadeIn(200);
              } else {
                  tooltip.hide();
              }
          });

          placeholder.bind("plothovercleanup", function (event, pos, item) {
              tooltip.hide();
          });

          var yaxes = [];
          for (var i=0; i<=max_yaxis; i++) {
              var position;
              if (i > 0) {
                  position = "right";
              }

              yaxes.push({
                  autoScale: "loose",
                  autoScaleMargin: 0.2,
                  position: position,
              });
          }

          var options = {
              grid: {
                  hoverable: true,
                  clickable: true,
              },
              series: {
                  lines: {
                      show: true
                  },
                  points: {
                      show: false
                  }
              },
              xaxes: [{
                  mode: params["xaxis_mode"] || "linear",
                  autoScale: "exact",
                  tickDecimals: 0
              }],
              yaxes: yaxes,
              selection: {
                  mode: "x"
              }
          };

          plot = $.plot(placeholder, series , options);
          placeholder.show();
      },
      controller: LineChartController,
      controllerAs: 'controller',
  };
};


/**
 * Name of the directive in Angular.
 *
 * @const
 * @export
 */
exports.LineChartDirective.directive_name = 'grrLineChart';
