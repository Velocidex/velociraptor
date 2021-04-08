import './line-charts.css';

import React from 'react';
import PropTypes from 'prop-types';
import _ from 'lodash';
import { ReferenceArea, ResponsiveContainer, LineChart,
         Line, CartesianGrid, XAxis, YAxis, Tooltip } from 'recharts';

const strokes = [
    "#ff7300", "#f48f8f", "#207300", "#f4208f"
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

          let now = new Date();
          let x_column = this.props.columns[0];
          let value = payload[0].payload[x_column];
          try {
              now.setTime(value * 1000);
              value = now.toISOString();
          } catch(e) {}

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
        stroke: PropTypes.number,
        payload: PropTypes.object
    }

    render() {
        return  (
            <g transform={`translate(${this.props.x},${this.props.y})`}>
              <text x={0} y={0} dy={16} textAnchor="end" fill="#666" transform="rotate(-35)">
                {this.props.payload.value}</text>
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
    }

    render() {
        let date = new Date();
        date.setTime(this.props.payload.value * 1000);

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
              <text x={0} y={0} dy={16} textAnchor="end" fill="#666" transform="rotate(-15)">
                {value}
              </text>
            </g>
        );
    }
}


export default class VeloLineChart extends React.Component {
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

        // Find the index into data corresponding to the first and
        // last x_column value.
        let refData = _.filter(this.props.data, x => x[x_column] >= from && x[x_column] <= to);
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

        return [refData[0][x_column], refData[refData.length-1][x_column],
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

    render() {
        let columns = this.props.columns;
        if (_.isEmpty(columns)) {
            return <div>No data</div>;
        }

        let x_column = columns[0];
        let tick_renderer = <DefaultTickRenderer />;
        let xaxis_mode = this.props.params && this.props.params.xaxis_mode;
        if (xaxis_mode === "time") {
            tick_renderer = <TimeTickRenderer
                              data={this.props.data}
                              dataKey={x_column}/>;
        }

        let lines = [];
        for(let i = 1; i<columns.length; i++) {
            let stroke = strokes[strokes.length % (i-1)];
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
                  data={this.props.data}
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
                         domain={[this.state.left, this.state.right]}
                         tick={tick_renderer}
                  />
                  <YAxis
                    allowDataOverflow
                    domain={[this.state.bottom, this.state.top]}
                  />
                  <Tooltip content={<CustomTooltip
                                      data={this.props.data}
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
