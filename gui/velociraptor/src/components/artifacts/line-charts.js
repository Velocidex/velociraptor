import './line-charts.css';

import React from 'react';
import PropTypes from 'prop-types';
import _ from 'lodash';
import { ReferenceArea, ResponsiveContainer,
         LineChart, BarChart, ScatterChart,
         Legend,
         Bar, Line, Scatter,
         CartesianGrid, XAxis, YAxis, Tooltip } from 'recharts';

import { ToStandardTime } from '../utils/time.js';

const strokes = [
    "#ff7300", "#f48f8f", "#207300", "#f4208f"
];

const shapes = [
    "star", "triangle", "circle",
];

class CustomTooltip extends React.Component {
  static propTypes = {
      type: PropTypes.string,
      payload: PropTypes.array,
      columns: PropTypes.array,
      data: PropTypes.array,
  }

  render() {
      const { active } = this.props;
      if (active) {
          const { payload } = this.props;
          let items = _.map(payload, (entry, i) => {
              let style = {
                  color: entry.color || '#000',
              };
              let { name, value } = entry;
              return (
                  <tr key={`tooltip-item-${i}`}>
                    <td className="" style={style}>{name}</td>
                    <td className="" style={style}>{value}</td>
                  </tr>
              );
          });

          let x_column = this.props.columns[0];
          let value = payload[0].payload[x_column];
          let now = ToStandardTime(value);
          if (_.isNaN(now)) {
              value = "";
          } else if(_.isDate(now)) {
              value = now.toISOString();
          }

          return (
              <>
                <table className="custom-tooltip">
                  <tbody>
                    <tr><td colSpan="2">{value}</td></tr>
                    {items}
                  </tbody>
                </table>
              </>
          );
      }
      return null;
  }
};

class DefaultTickRenderer extends React.Component {
    static propTypes = {
        x: PropTypes.number,
        y: PropTypes.number,
        stroke: PropTypes.string,
        payload: PropTypes.object,
        transform: PropTypes.string,
    }

    render() {
        let transform = this.props.transform;
        let value = this.props.payload.value;
        console.log(this.props.transform);
        if (_.isNumber(value)) {
            value = Math.round(value * 10) / 10;
        };

        return  (
            <g transform={`translate(${this.props.x},${this.props.y})`}>
              <text x={0} y={0} dy={16}
                    textAnchor="end" fill="#666"
                    transform={this.props.transform}>
                {value} {this.props.transform}
              </text>
            </g>
        );
    }
}


class TimeTickRenderer extends React.Component {
    static propTypes = {
        x: PropTypes.number,
        y: PropTypes.number,
        stroke: PropTypes.string,
        payload: PropTypes.object,

        data: PropTypes.array,
        dataKey: PropTypes.string,
        transform: PropTypes.string,
    }

    render() {
        let date = ToStandardTime(this.props.payload.value);
        if (_.isNaN(date) || _.isUndefined(date.getUTCFullYear)) {
            return <></>;
        }
        let first_date = this.props.data[0][this.props.dataKey];
        let last_date =  this.props.data[this.props.data.length -1][this.props.dataKey];

        let value = date.getUTCFullYear().toString().padStart(4, "0") + "-" +
            (date.getUTCMonth()+1).toString().padStart(2, "0") + "-" +
            date.getUTCDate().toString().padStart(2, "0");

        if (last_date - first_date < 24 * 60 * 60) {
            value = date.getUTCHours().toString().padStart(2, "0") + ":" +
                date.getUTCMinutes().toString().padStart(2, "0");
        }

        return  (
            <g transform={`translate(${this.props.x},${this.props.y})`}>
              <text x={0} y={0} dy={16}
                    textAnchor="end" fill="#666"
                    transform={this.props.transform}>
                {value}
              </text>
            </g>
        );
    }
}

export class VeloLineChart extends React.Component {
    static propTypes = {
        params: PropTypes.object,
        columns: PropTypes.array,
        data: PropTypes.array,
    };

    state = {
        data: [],
        left: 'dataMin',
        right: 'dataMax',
        refAreaLeft: '',
        refAreaRight: '',
        top: 'dataMax+1',
        bottom: 'dataMin-1',
        animation: true,
    }

    getAxisYDomain = (from, to, ref, offset) => {
        let x_column = this.props.columns[0];
        let from_factor = 1;
        if (from > 2042040202) {
            from_factor = 1000;
        }

        let data_factor = 1;
        if (this.props.data.length === 0) {
            return [0, 0, 0, 0];
        }
        if (this.props.data[0][x_column] > 2042040202) {
            data_factor = 1000;
        }

        let factor = from_factor / data_factor;

        // Find the index into data corresponding to the first and
        // last x_column value.
        let refData = _.filter(this.props.data,
                               x => x[x_column] >= from / factor &&
                               x[x_column] <= to / factor);
        if (_.isEmpty(refData)) {
            return [0, 0, 0, 0];
        }

        // Find the top or bottom value that is largest of all
        // columns.
        let first_value = refData[0][this.props.columns[1]];
        let [bottom, top] = [first_value, first_value];
        /* eslint no-loop-func: "off" */
        for (let i = 1; i<this.props.columns.length; i++) {
            let column = this.props.columns[i];
            refData.forEach((d) => {
                if (d[column] > top) top = d[column];
                if (d[column] < bottom) bottom = d[column];
            });
        }

        let range = top-bottom;

        return [refData[0][x_column] * factor,
                refData[refData.length-1][x_column] * factor,
                bottom - 0.1 * range, top + 0.1 * range];
    };

    zoom = () => {
        let { refAreaLeft, refAreaRight } = this.state;
        if (refAreaLeft === refAreaRight || !refAreaRight) {
            this.setState(() => ({
                refAreaLeft: '',
                refAreaRight: '',
            }));
            return;
        }

        // xAxis domain
        if (refAreaLeft > refAreaRight) [refAreaLeft, refAreaRight] = [refAreaRight, refAreaLeft];

        // yAxis domain
        const [left, right, bottom, top] = this.getAxisYDomain(refAreaLeft, refAreaRight, 1);

        this.setState(() => ({
            refAreaLeft: '',
            refAreaRight: '',
            left: left,
            right: right,
            bottom: bottom,
            top: top,
        }));
    }

    zoomOut = () => {
        this.setState(() => ({
            refAreaLeft: '',
            refAreaRight: '',
            left: 'dataMin',
            right: 'dataMax',
            top: 'dataMax+1',
            bottom: 'dataMin',
        }));
    }

    // Force all data to numbers.
    sanitizeData = data=>{
        if (_.isEmpty(this.props.columns)) {
            return data;
        };
        let xaxis_mode = this.props.params && this.props.params.xaxis_mode;
        let x_column = this.props.columns[0];

        return _.map(data, row=>{
            let new_row = {};
            _.each(row, (value, key)=>{
                if (xaxis_mode && key === x_column) {
                    let date = ToStandardTime(value);
                    if (_.isDate(date)) {
                        new_row[key] = date.getTime();
                    }
                } else {
                    new_row[key] = _.toFinite(value);
                }
            });
            return new_row;
        });
    }

    render() {
        let data = this.sanitizeData(this.props.data);
        let columns = this.props.columns;
        if (_.isEmpty(columns)) {
            return <div>No data</div>;
        }

        let x_column = columns[0];
        let tick_renderer = <DefaultTickRenderer transform="rotate(-35)" />;
        let y_tick_renderer = <DefaultTickRenderer />;
        let xaxis_mode = this.props.params && this.props.params.xaxis_mode;
        if (xaxis_mode === "time") {
            tick_renderer = <TimeTickRenderer
                              data={data}
                              transform="rotate(-35)"
                              dataKey={x_column}/>;
        }

        let lines = [];
        for(let i = 1; i<columns.length; i++) {
            let stroke = strokes[(i-1) % strokes.length];
            lines.push(
                <Line type="monotone"
                      dataKey={columns[i]} key={i}
                      stroke={stroke}
                      animationDuration={300}
                      dot={false} />);
        }
        return (
            <div onDoubleClick={this.zoomOut} >
              <ResponsiveContainer width="95%"
                                   height={600} >
                <LineChart
                  className="velo-line-chart"
                  data={data}
                  onMouseDown={e => {
                      let label = e && e.activeLabel;
                      if (!label) {
                          return;
                      }
                      if (this.state.refAreaLeft) {
                          this.setState({ refAreaRight: label });
                          this.zoom();
                      } else {
                          this.setState({ refAreaLeft: label });
                      }
                  }}
                  onMouseMove={e => {
                      let refAreaRight = e && e.activeLabel;
                      if (this.state.refAreaLeft && refAreaRight) {
                          this.setState({refAreaRight: refAreaRight});
                      }
                  }}
                  onMouseUp={this.zoom}
                  margin={{ top: 5, right: 20, left: 10, bottom: 75 }}
                >
                  <XAxis dataKey={x_column}
                         type="number"
                         allowDataOverflow
                         domain={[this.state.left || 0, this.state.right || 0]}
                         tick={tick_renderer}
                  />
                  <YAxis
                    allowDataOverflow
                    domain={[this.state.bottom || 0, this.state.top || 0]}
                    tick={y_tick_renderer}
                  />
                  <Tooltip content={<CustomTooltip
                                      data={data}
                                      columns={this.props.columns}/>} />
                  {lines}
                  <CartesianGrid stroke="#f5f5f5" />
                  {
                      (this.state.refAreaLeft && this.state.refAreaRight) ? (
                          <ReferenceArea x1={this.state.refAreaLeft}
                                         x2={this.state.refAreaRight} strokeOpacity={0.3} />) : null
                  }

                </LineChart>
              </ResponsiveContainer>
            </div>
        );
    }
};


// Time charts receive one or more columns - the first column is a
// time while each other column is data.
export class VeloTimeChart extends VeloLineChart {
    // Force all data to numbers.
    sanitizeData = data=>{
        if (_.isEmpty(this.props.columns)) {
            return data;
        };
        let x_column = this.props.columns[0];

        return _.map(data, row=>{
            let new_row = {};
            _.each(row, (value, key)=>{
                // The x column must be converted to time.
                if (key === x_column) {
                    let date = ToStandardTime(value);
                    if (_.isDate(date)) {
                        new_row[key] = date.getTime();
                    }
                } else {
                    new_row[key] = _.toFinite(value);
                }
            });
            return new_row;
        });
    }

    render() {
        let data = this.sanitizeData(this.props.data);
        let columns = this.props.columns;
        if (_.isEmpty(columns)) {
            return <div>No data</div>;
        }

        let x_column = columns[0];
        let tick_renderer = <TimeTickRenderer
                              data={data}
                              transform="rotate(-35)"
                              dataKey={x_column}/>;

        let lines = _.map(this.props.columns, (column, i)=>{
            if (i===0) {
                return <div key={i}></div>;
            };
            let stroke = strokes[(i-1) % strokes.length];
            return <Line type="monotone"
                         dataKey={columns[i]} key={i}
                         stroke={stroke}
                         animationDuration={300}
                         dot={false} />;
        });

        return (
            <div onDoubleClick={this.zoomOut} >
              <ResponsiveContainer width="95%"
                                   height={600} >
                <LineChart
                  className="velo-line-chart"
                  data={data}
                  onMouseDown={e => {
                      let label = e && e.activeLabel;
                      if (!label) {
                          return;
                      }
                      if (this.state.refAreaLeft) {
                          this.setState({ refAreaRight: label });
                          this.zoom();
                      } else {
                          this.setState({ refAreaLeft: label });
                      }
                  }}
                  onMouseMove={e => {
                      let refAreaRight = e && e.activeLabel;
                      if (this.state.refAreaLeft && refAreaRight) {
                          this.setState({refAreaRight: refAreaRight});
                      }
                  }}
                  onMouseUp={this.zoom}
                  margin={{ top: 5, right: 20, left: 10, bottom: 75 }}
                >
                  <XAxis dataKey={x_column}
                         type="number"
                         allowDataOverflow
                         domain={[this.state.left || 0, this.state.right || 0]}
                         tick={tick_renderer}
                  />
                  <YAxis
                    allowDataOverflow
                    domain={[this.state.bottom || 0, this.state.top || 0]}
                  />
                  <Tooltip content={<CustomTooltip
                                      data={data}
                                      columns={this.props.columns}/>} />
                  {lines}
                  <CartesianGrid stroke="#f5f5f5" />
                  {
                      (this.state.refAreaLeft && this.state.refAreaRight) ? (
                          <ReferenceArea x1={this.state.refAreaLeft}
                                         x2={this.state.refAreaRight} strokeOpacity={0.3} />) : null
                  }

                </LineChart>
              </ResponsiveContainer>
            </div>
        );
    }
}

export class VeloBarChart extends VeloLineChart {
    // Force all data to numbers, except for the X axis which are
    // strings.
    sanitizeData = data=>{
        if (_.isEmpty(this.props.columns)) {
            return data;
        };


        // The first column is the label
        let x_column = this.props.columns[0];

        return _.map(data, (row, idx)=>{
            // The x column must be converted to a string since it
            // is the label.
            let new_row = {
                _name: _.toString(row[x_column]),
            };
            _.each(row, (value, key)=>{
                if (key !== x_column) {
                    new_row[key] = _.toFinite(value);
                }
            });
            return new_row;
        });
    }

    render() {
        if (_.isEmpty(this.props.columns)) {
            return <div>No data</div>;
        }

        let chart_type = this.props.params && this.props.params.type;
        let data = this.sanitizeData(this.props.data);

        let lines =  _.map(this.props.columns, (column, i)=>{
            // First column is the name, we do not build a line for it.
            if (i===0) {
                return <div key={i}></div>;
            }

            let fill = strokes[i % strokes.length];
            let stackid = null;
            if (chart_type === "stacked") {
                stackid = "A";
            }
            return <Bar type="monotone"
                     dataKey={column} key={i}
                     fill={fill}
                     stroke={fill}
                     stackId={stackid}
                     animationDuration={300}
                   />;
        });

        return (
            <ResponsiveContainer width="95%"  height={600}>
              <BarChart
                className="velo-line-chart"
                data={data}
              >
                <CartesianGrid strokeDasharray="3 3" />
                <XAxis dataKey="_name" />
                <YAxis />
                <Tooltip />
                <Legend />
                {lines}
              </BarChart>
            </ResponsiveContainer>
        );
    };
}

export class VeloScatterChart extends VeloLineChart {
    sanitizeData = data=>{
        if (_.isEmpty(this.props.columns)) {
            return data;
        };

        let name_column = this.props.params && this.props.params.name_column;

        // All columns must be numbers.
        return _.map(data, (row, idx)=>{
            let new_row = {};
            _.each(row, (value, key)=>{
                if (key !== name_column) {
                    new_row[key] = _.toFinite(value);
                } else {
                    new_row[key] = _.toString(value);
                }
            });
            return new_row;
        });
    }

    render() {
        if (_.isEmpty(this.props.columns)) {
            return <div>No data</div>;
        }

        let name_column = this.props.params && this.props.params.name_column;
        if (!name_column) {
            name_column = this.props.columns[0];
        };
        let data = this.sanitizeData(this.props.data);
        let lines = _.map(this.props.columns, (column, i)=>{
            if (column === name_column) {
                return <div key={i}></div>;
            }
            let fill = strokes[i % strokes.length];
            let shape = shapes[i % shapes.length];
            return <Scatter type="monotone"
                         dataKey={column} key={i}
                         fill={fill}
                         stroke={fill}
                         shape={shape}
                         animationDuration={300}
                   />;
        });

        return (
            <ResponsiveContainer width="95%"  height={600}>
              <ScatterChart
                className="velo-line-chart"
                data={data}
              >
                <CartesianGrid strokeDasharray="3 3" />
                <XAxis dataKey={name_column} />
                <YAxis />
                <Tooltip />
                <Legend />
                {lines}
              </ScatterChart>
            </ResponsiveContainer>
        );
    };
}
