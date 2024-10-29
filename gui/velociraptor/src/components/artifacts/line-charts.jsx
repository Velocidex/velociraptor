import './line-charts.css';

import React from 'react';
import PropTypes from 'prop-types';
import _ from 'lodash';
import { ReferenceArea, ResponsiveContainer,
         LineChart, BarChart, ScatterChart,
         Legend,
         Bar, Line, Scatter,
         CartesianGrid, XAxis, YAxis, Tooltip } from 'recharts';

import { ToStandardTime, FormatRFC3339 } from '../utils/time.jsx';
import UserConfig from '../core/user.jsx';
import moment from 'moment';
import ToolTip from '../widgets/tooltip.jsx';
import T from '../i8n/i8n.jsx';
import VeloValueRenderer from '../utils/value.jsx';

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
      active: PropTypes.any,
    }

    render() {
        if (!this.props.active || _.isEmpty(this.props.payload)) {
            return <></>;
        };
        let items = _.map(this.props.payload, (entry, i) => {
            let style = {
                color: entry.color || '#000',
            };
            let { name, value } = entry;
            return (
                <tr key={i}>
                  <td className="" style={style}>{name}</td>
                  <td className="" style={style}>{value}</td>
                </tr>
            );
        });

        let x_column = this.props.columns[0];
        let first_series = this.props.payload[0];
        let value = first_series.payload && first_series.payload[x_column];

        return (
            <>
              <table className="custom-tooltip">
                <tbody>
                  <tr>
                    <td>{x_column}</td>
                    <td>
                      <VeloValueRenderer value={value} />
                    </td>
                  </tr>
                  {items}
                </tbody>
              </table>
            </>
        );
  }
};


class TimeTooltip extends React.Component {
    static contextType = UserConfig;

    static propTypes = {
      type: PropTypes.string,
      payload: PropTypes.array,
      columns: PropTypes.array,
      data: PropTypes.array,
      active: PropTypes.any,
    }

    fromLocalTZ = ts=>{
        let timezone = this.context.traits.timezone || "UTC";
        let zone = moment.tz.zone(timezone);
        if (!zone) {
            return ts;
        }
        return moment.utc(ts).add(zone.utcOffset(ts), "minutes").valueOf();
    }

    render() {
        if (!this.props.active || _.isEmpty(this.props.payload)) {
            return <></>;
        };
        let items = _.map(this.props.payload, (entry, i) => {
            let style = {
                color: entry.color || '#000',
            };
            let { name, value } = entry;
            return (
                <tr key={i}>
                  <td className="" style={style}>{name}</td>
                  <td className="" style={style}>{value}</td>
                </tr>
            );
        });

        let x_column = this.props.columns[0];
        let first_time = this.props.payload[0];
        let value = first_time.payload && first_time.payload[x_column];
        let now = ToStandardTime(value);
        let timezone = this.context.traits.timezone || "UTC";
        let formatted_value = FormatRFC3339(this.fromLocalTZ(now), timezone);

        return (
            <>
              <table className="custom-tooltip">
                <tbody>
                  <tr><td colSpan="2">{formatted_value}</td></tr>
                  {items}
                </tbody>
              </table>
            </>
        );
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
        let value = this.props.payload.value;
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
    static contextType = UserConfig;

    static propTypes = {
        x: PropTypes.number,
        y: PropTypes.number,
        stroke: PropTypes.string,
        payload: PropTypes.object,

        data: PropTypes.array,
        dataKey: PropTypes.string,
        transform: PropTypes.string,
    }

    fromLocalTZ = ts=>{
        let timezone = this.context.traits.timezone || "UTC";
        let zone = moment.tz.zone(timezone);
        if (zone) {
            ts = moment.utc(ts).add(zone.utcOffset(ts), "minutes");
        }
        return {
            str: FormatRFC3339(ts, timezone),
            short: moment.tz(ts, timezone).format('YYYY-MM-DD'),
        };
    }

    render() {
        let ts = this.fromLocalTZ(this.props.payload.value);
        return  (
            <ToolTip tooltip={ts.str}>
              <g transform={`translate(${this.props.x},${this.props.y})`}>
                <text x={0} y={0} dy={16}
                      textAnchor="end" fill="#666"
                      transform={this.props.transform}>
                  {ts.short}
                </text>
              </g>
            </ToolTip>
        );
    }
}

export class VeloLineChart extends React.Component {
    static propTypes = {
        params: PropTypes.object,
        columns: PropTypes.array,
        data: PropTypes.array,
    };

    componentDidMount = () => {
        this.syncData();
    }

    componentDidUpdate = (prevProps, prevState, rootNode) => {
        if(this.state.data.length != this.props.data.length) {
            this.syncData();
        }
    }

    // Sync the props data into the state after transforming it.
    syncData = ()=>{
        if (_.isEmpty(this.props.columns)) {
            return;
        }

        let data = [];
        let x_column = this.props.columns[0];
        for(let i=0;i<this.props.data.length;i++){
            let data_pt = Object.assign({}, this.props.data[i]);
            data_pt[x_column] = this.toLocalX(data_pt[x_column]);
            data.push(data_pt);
        }
        this.setState({data: data,
                       x_column: x_column,
                       columns: this.props.columns});
    }

    state = {
        // A local copy of the transformed data
        data: [],
        x_column: "",
        columns: [],

        // Left and right boundaries of the selection zone.
        refAreaLeft: '',
        refAreaRight: '',

        // The viewpoint x axis references - in local timezone.
        left: 'dataMin',
        right: 'dataMax',
        top: 'dataMax+1',
        bottom: 'dataMin-1',
    }

    toLocalX = x=>{
        if(!_.isNumber(x)) {
            return 0;
        }

        return x;
    }

    getAxisYDomain = (from, to, ref, offset) => {
        let left = 1e64;
        let right = -1e64;
        let top = -1e64;
        let bottom = 1e64;

        let x_column = this.state.columns[0];
        if (this.state.data.length === 0 ||
            this.state.columns.length < 2) {
            return [0, 0, 0, 0];
        }

        for (let i=0; i<this.state.data.length; i++) {
            let data_pt = this.state.data[i];
            let x = data_pt[x_column];
            if (x < from || x > to) {
                continue;
            }
            if(x > right) {
                right = x;
            }
            if (x < left) {
                left = x;
            }

            for(let j=1;j<this.state.columns.length;j++) {
                let y_column = this.state.columns[j];
                let y = data_pt[y_column];
                if(y > top) {
                    top = y;
                }
                if (y < bottom) {
                    bottom = y;
                }
            }
        }
        return [left, right, bottom, top];
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

        // xAxis domain - make sure it is the correct orientation
        if (refAreaLeft > refAreaRight) {
            [refAreaLeft, refAreaRight] = [refAreaRight, refAreaLeft];
        }

        // yAxis domain
        const [left, right, bottom, top] = this.getAxisYDomain(
            refAreaLeft, refAreaRight, 1);

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

    render() {
        let data = this.state.data;
        let columns = this.state.columns;
        if (_.isEmpty(columns) || _.isEmpty(data)) {
            return <div>{T("No data")}</div>;
        }

        let x_column = columns[0];
        let lines = [];
        for(let i = 1; i<columns.length; i++) {
            let stroke = strokes[(i-1) % strokes.length];
            lines.push(
                <Line type="monotone"
                      dataKey={columns[i]} key={i}
                      stroke={stroke}
                      isAnimationActive={true}
                      animationDuration={300}
                      dot={false} />);
        }

        if(_.isEmpty(lines)) {
            return <div>{T("No data")}</div>;
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
                         tick={<DefaultTickRenderer transform="" />}
                  />
                  <YAxis
                    allowDataOverflow
                    domain={[this.state.bottom || 0, this.state.top || 0]}
                    tick={<DefaultTickRenderer />}
                  />
                  <Tooltip content={
                      <CustomTooltip
                        data={data}
                        columns={this.props.columns}/>} />
                  {lines}
                  <CartesianGrid stroke="#f5f5f5" />
                  {this.state.refAreaLeft && this.state.refAreaRight &&
                   <ReferenceArea x1={this.state.refAreaLeft}
                                  x2={this.state.refAreaRight} strokeOpacity={0.3} />
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
    static contextType = UserConfig;

    // Times must be shown in the local timezone (chosen by the user),
    // so we store them internally as such.
    toLocalX = ts=>{
        ts = ToStandardTime(ts);
        let timezone = this.context.traits.timezone || "UTC";
        let zone = moment.tz.zone(timezone);
        if (!zone) {
            return ts;
        }
        return moment.utc(ts).subtract(zone.utcOffset(ts), "minutes").valueOf();
    }

    render() {
        let data = this.state.data;
        let columns = this.state.columns;
        if (_.isEmpty(columns) || _.isEmpty(data)) {
            return <div>{T("No data")}</div>;
        }

        let x_column = columns[0];
        let lines = _.map(this.state.columns, (column, i)=>{
            if (i===0) {
                return <div key={i}></div>;
            };
            let stroke = strokes[(i-1) % strokes.length];
            return <Line type="monotone"
                         dataKey={columns[i]} key={i}
                         stroke={stroke}
                         isAnimationActive={true}
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
                         tick={<TimeTickRenderer
                              data={data}
                              transform="rotate(-35)"
                              dataKey={x_column}/>}
                  />
                  <YAxis
                    allowDataOverflow
                    domain={[this.state.bottom || 0, this.state.top || 0]}
                  />
                  <Tooltip content={
                      <TimeTooltip
                        data={data}
                        columns={this.state.columns}/>} />
                  {lines}
                  <CartesianGrid stroke="#f5f5f5" />
                  {this.state.refAreaLeft && this.state.refAreaRight &&
                   <ReferenceArea x1={this.state.refAreaLeft}
                                  x2={this.state.refAreaRight}
                                  strokeOpacity={0.3} />
                  }
                </LineChart>
              </ResponsiveContainer>
            </div>
        );
    }
}

export class VeloBarChart extends VeloLineChart {
    toLocalX = x=>_.toString(x);

    render() {
        let data = this.state.data;
        let columns = this.state.columns;
        if (_.isEmpty(columns) || _.isEmpty(data)) {
            return <div>{T("No data")}</div>;
        }

        let chart_type = this.props.params && this.props.params.type;
        let lines =  _.map(this.state.columns, (column, i)=>{
            // First column is the name, we do not build a line for
            // it.
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
                        isAnimationActive={true}
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
    syncData = ()=>{
        if (_.isEmpty(this.props.columns)) {
            return;
        }

        let data = [];
        let name_column = this.props.params && this.props.params.name_column;
        if (!name_column) {
            name_column = this.props.columns[0];
        }

        for(let i=0;i<this.props.data.length;i++){
            let data_pt = Object.assign({}, this.props.data[i]);
            data_pt[name_column] = _.toString(data_pt[name_column]);
            data.push(data_pt);
        };

        this.setState({data: data,
                       columns: this.props.columns});
    }

    render() {
        let data = this.state.data;
        let columns = this.state.columns;
        if (_.isEmpty(columns) || _.isEmpty(data)) {
            return <div>{T("No data")}</div>;
        }

        let name_column = this.props.params && this.props.params.name_column;
        if (!name_column) {
            name_column = this.props.columns[0];
        };
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
