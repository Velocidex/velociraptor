import "./timeline.css";

import React, { Component } from 'react';
import PropTypes from 'prop-types';
import _ from 'lodash';
import Timeline, {
    TimelineMarkers,
    CustomMarker,
} from 'react-calendar-timeline';
import api from '../core/api-service.jsx';
import {CancelToken} from 'axios';
import { PrepareData } from '../core/table.jsx';
import VeloTimestamp from "../utils/time.jsx";
import VeloValueRenderer from '../utils/value.jsx';
import Form from 'react-bootstrap/Form';
import { JSONparse } from '../utils/json_parse.jsx';

// make sure you include the timeline stylesheet or the timeline will not be styled
import 'react-calendar-timeline/lib/Timeline.css';
import moment from 'moment';
import 'moment-timezone';
import Button from 'react-bootstrap/Button';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import ButtonGroup from 'react-bootstrap/ButtonGroup';
import Navbar from 'react-bootstrap/Navbar';
import T from '../i8n/i8n.jsx';
import Table from 'react-bootstrap/Table';
import ToolTip from '../widgets/tooltip.jsx';
import { ColumnToggle } from '../core/paged-table.jsx';

const FixedColumns = {
    "Time": 1,
    "Desc": 1,
    "Message": 1,
    "Data": 1,
    "TimestampDesc": 1,
}

class TimelineTableRow extends Component {
    static propTypes = {
        row: PropTypes.object,
        columns: PropTypes.array,
        timeline_class: PropTypes.string,
    }

    state = {
        expanded: false
    }

    renderCell = (column, rowIdx) => {
        let data = this.props.row.Data || {};
        let cell = this.props.row[column] || data[column] || "";
        if(column === "Time") {
            return <td key={column}>
                     <div className={this.props.timeline_class}>
                       <VeloTimestamp usec={cell}/>
                     </div>
                   </td>;
        }
        return <td key={column}> <VeloValueRenderer value={cell}/> </td>;
    };

    render() {
        let data = this.props.row["Data"] || {};
        let row_class = "timeline-data ";
        if(!this.state.expanded) {
            row_class += "hidden";
        }
        return (
            <React.Fragment >
              <tr className="row-selected"
                  onClick={e=>this.setState({expanded: !this.state.expanded})}
                >
                {_.map(this.props.columns, this.renderCell)}
              </tr>
              <tr className={row_class}>
                <td colSpan="30">
                  <VeloValueRenderer value={data} />
                </td>
              </tr>
            </React.Fragment>
        );
    }
}



class TimelineTableRenderer  extends Component {
    static propTypes = {
        rows: PropTypes.array,
        timelines: PropTypes.object,
        extra_columns: PropTypes.array,
    }

    getTimelineClass = (name) => {
        let timelines = this.props.timelines.timelines;
        if (_.isArray(timelines)) {
            for(let i=0;i<timelines.length;i++) {
                if (timelines[i].id === name) {
                    return "timeline-item-" + (i + 1);
                };
            }
        }
        return "";
    }

    columns = ["Time", "Desc", "Message"];

    renderRow = (row, idx)=>{
        let columns = this.columns.concat(this.props.extra_columns);
        return (
            <TimelineTableRow key={idx}
              timeline_class={this.getTimelineClass(_.toString(row._Source))}
              row={row}
              columns={columns}
            />
        );
    }

    render() {
        if (_.isEmpty(this.props.rows)) {
            return <div className="no-content velo-table">{T("No events")}</div>;
        }

        return <Table className="paged-table">
                <thead>
                  <tr className="paged-table-header">
                    {_.map(this.columns, (x, i)=>{
                        return <th key={i} className={i == 0 ? "time" : ""}>
                                 { T(x) }
                               </th>;
                    })}
                    {_.map(this.props.extra_columns || [], (x, i)=>{
                        return <th key={i}>
                                 { x }
                               </th>;
                    })}
                  </tr>
                </thead>
                 <tbody className="fixed-table-body">
                   {_.map(this.props.rows, this.renderRow)}
                 </tbody>
               </Table>;
    }
}


export default class TimelineRenderer extends React.Component {
    static propTypes = {
        name: PropTypes.string,
        notebook_id: PropTypes.string,
        params: PropTypes.object,
    }

    componentDidMount = () => {
        this.source = CancelToken.source();
        this.fetchRows();
    }

    componentWillUnmount() {
        this.source.cancel();
    }

    componentDidUpdate(prevProps, prevState, snapshot) {
        if (!_.isEqual(prevState.version, this.state.version)) {
            return true;
        }

        if (!_.isEqual(prevState.start_time, this.state.start_time)) {
            this.fetchRows();
            return true;
        };

        if (!_.isEqual(prevState.row_count, this.state.row_count)) {
            this.fetchRows();
            return true;
        };

        return false;
    }

    handleTimeChange = (visibleTimeStart, visibleTimeEnd) => {
        this.setState({
            visibleTimeStart,
            visibleTimeEnd,
            scrolling: true
        });
    };

    state = {
        start_time: 0,
        table_start: 0,
        table_end: 0,
        loading: true,
        disabled: {},
        version: 0,
        row_count: 10,
        visibleTimeStart: 0,
        visibleTimeEnd: 0,
        toggles: {},
    };

    fetchRows = () => {
        let skip_components = [];
        _.map(this.state.disabled, (v,k)=>{
            if(v) {
                skip_components.push(k);
            };
        });

        let start_time = this.state.start_time * 1000000;
        if (start_time < 1000000000) {
            start_time = 0;
        }

        let params = {
            type: "TIMELINE",
            timeline: this.props.name,
            start_time: start_time,
            rows: this.state.row_count,
            skip_components: skip_components,
            notebook_id: this.props.notebook_id,
        };

        let url = "v1/GetTable";

        this.source.cancel();
        this.source = CancelToken.source();

        this.setState({loading: true});

        api.get(url, params, this.source.token).then((response) => {
            if (response.cancel) {
                return;
            }
            let start_time = (response.data.start_time / 1000000) || 0;
            let pageData = PrepareData(response.data);
            this.setState({
                table_start: start_time,
                table_end:  response.data.end_time / 1000000 || 0,
                columns: pageData.columns,
                rows: pageData.rows,
                version: Date(),
            });

            if (this.state.visibleTimeStart === 0) {
                let visibleTimeStart = start_time || Date.now();
                let visibleTimeEnd = visibleTimeStart + 60 * 60 * 10000;
                this.setState({visibleTimeStart: visibleTimeStart,
                               visibleTimeEnd: visibleTimeEnd});
            }

            this.updateToggles(pageData.rows);
        });
    };

    groupRenderer = ({ group }) => {
        if (group.id < 0) {
            return <div>{group.title}</div>;
        }

        return (
            <Form>
              <Form.Check
                className="custom-group"
                type="checkbox"
                label={group.title}
                checked={!group.disabled}
                onChange={()=>{
                    let disabled = this.state.disabled;
                    disabled[group.id] = !disabled[group.id];
                    this.setState({disabled: disabled});
                    this.fetchRows();
                }}
              />
            </Form>
        );
    };

    pageSizeSelector = () => {
        let options = [10, 20, 50, 100];

        return <div className="btn-group" role="group">
                 { _.map(options, option=>{
                     return <button
                              key={ option }
                              type="button"
                              onClick={()=>this.setState({row_count: option})}
                              className={ `btn ${option === this.state.row_count ? 'btn-secondary' : 'btn-default'}` }
                            >
                              { option }
                            </button>;
                     }) }
               </div>;
    }

    nextPage = ()=>{
        if (this.state.table_end > 0) {
            this.setState({start_time: this.state.table_end + 1});
        }
    }

    updateToggles = rows=>{
        // Find all unique columns
        let _columns={};
        let columns = [];
        let toggles = {...this.state.toggles};

        _.each(this.state.rows, row=>{
            let data = row.Data || {};
            _.each(data, (v, k)=>{
                if (_.isUndefined(_columns[k]) && !FixedColumns[k]) {
                    _columns[k]=1;
                    columns.push(k);

                    if(_.isUndefined(toggles[k])) {
                        toggles[k] = true;
                    }
                }
            });
        });

        this.setState({toggles: toggles, columns: columns});
    }

    renderColumnSelector = ()=>{
        return (
            <ColumnToggle
              columns={this.state.columns}
              toggles={this.state.toggles}
              onToggle={c=>{
                  if(c) {
                      let toggles = this.state.toggles;
                      toggles[c] = !toggles[c];
                      this.setState({toggles: toggles});
                  }
              }}
            />
        );
    }

    render() {
        let super_timeline = {timelines:[]};
        if (_.isString(this.props.params)) {
            super_timeline = JSONparse(this.props.params);
            if(!super_timeline) {
                return <></>;
            }
        } else if(_.isObject(this.props.params)) {
            super_timeline = this.props.params;
        }

        let groups = [{id: -1, title: "Table View"}];
        let items = [{
            id:-1, group: -1,
            start_time: this.state.table_start,
            end_time: this.state.table_end,
            canMove: false,
            canResize: false,
            canChangeGroup: false,
            itemProps: {
                className: 'timeline-table-item',
                style: {
                    background: undefined,
                    color: undefined,
                },
            },
        }];
        let smallest = 10000000000000000;
        let largest = 0;
        let timelines = super_timeline.timelines || [];

        for (let i=0;i<timelines.length;i++) {
            let timeline = super_timeline.timelines[i];
            let start = timeline.start_time / 1000000;
            let end = timeline.end_time / 1000000;
            if (start < smallest) {
                smallest = start;
            }

            if (end > largest) {
                largest = end;
            }

            groups.push({
                id: timeline.id,
                disabled: this.state.disabled[timeline.id],
                title: timeline.id,
            });
            items.push({
                id: i+1, group: timeline.id,
                start_time: start,
                end_time: end,
                canMove: false,
                canResize: false,
                canChangeGroup: false,
                itemProps: {
                    className: 'timeline-item-' + ((i + 1) % 8),
                    style: {
                        background: undefined,
                        color: undefined,
                    }
                },
            });
        }

        if (smallest > largest) {
            smallest = largest;
        }

        if (_.isNaN(smallest)) {
            smallest = 0;
            largest =0;
        }

        let extra_columns = [];
        _.each(this.state.toggles, (v,k)=>{
            if(!v) { extra_columns.push(k); }});

        return <div className="super-timeline">Super-timeline {this.props.name}
                 <Navbar className="toolbar">
                   <ButtonGroup>
                     { this.renderColumnSelector() }
                     { this.pageSizeSelector() }
                     <Button title="Next"
                             onClick={() => {this.nextPage(); }}
                             variant="default">
                       <FontAwesomeIcon icon="forward"/>
                     </Button>
                   </ButtonGroup>
                 </Navbar>
                 <Timeline
                   groups={groups}
                   items={items}
                   defaultTimeStart={moment(smallest).add(-1, "day")}
                   defaultTimeEnd={moment(largest).add(1, "day")}
                   itemTouchSendsClick={true}
                   minZoom={5*60*1000}
                   dragSnap={1000}
                   onCanvasClick={(groupId, time, e) => {
                       this.setState({start_time: time});
                   }}
                   onItemSelect={(itemId, e, time) => {
                       this.setState({start_time: time});
                       return false;
                   }}
                   onItemClick={(itemId, e, time) => {
                       this.setState({start_time: time});
                       return false;
                   }}
                   groupRenderer={this.groupRenderer}
                   onTimeChange={this.handleTimeChange}
                   visibleTimeStart={this.state.visibleTimeStart}
                   visibleTimeEnd={this.state.visibleTimeEnd}
                 >
                   <TimelineMarkers>
                     <CustomMarker
                       date={this.state.start_time} >
                       { ({ styles, date }) => {
                           styles.backgroundColor = undefined;
                           styles.width = undefined;
                           return <div style={styles}
                                       className="timeline-marker"
                                  />;
                       }}
                     </CustomMarker>
                   </TimelineMarkers>
                 </Timeline>
                 { this.state.columns &&
                   <TimelineTableRenderer
                     timelines={super_timeline}
                     extra_columns={extra_columns}
                     rows={this.state.rows} />
                 }
               </div>;
    }
}
