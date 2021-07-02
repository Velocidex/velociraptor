import "./timeline.css";

import React, { Component } from 'react';
import PropTypes from 'prop-types';
import _ from 'lodash';
import Timeline, {
    TimelineMarkers,
    CustomMarker,
    TodayMarker,
    CursorMarker
} from 'react-calendar-timeline';
import VeloTable from '../core/table.js';
import api from '../core/api-service.js';
import axios from 'axios';
import { PrepareData } from '../core/table.js';
import VeloTimestamp from "../utils/time.js";
import VeloValueRenderer from '../utils/value.js';
import Form from 'react-bootstrap/Form';

// make sure you include the timeline stylesheet or the timeline will not be styled
import 'react-calendar-timeline/lib/Timeline.css';
import moment from 'moment';
import Button from 'react-bootstrap/Button';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';

const item_colors = ["white", "red",
                     "blue", "cyan"];

class TimelineValueRenderer extends Component {
    static propTypes = {
        value: PropTypes.object,
    }
    state = {
        expanded: false,
    }
    render() {
        return (
            <div className="timeline-value-container">
              { this.state.expanded ?
                <span>
                  <Button
                    variant="default-outline" size="sm"
                    onClick={()=>this.setState({expanded: false})} >
                    <FontAwesomeIcon icon="minus"/>
                  </Button>
                  <div className="timeline-value">
                    <VeloValueRenderer value={this.props.value}/>
                  </div>
                </span>
                :
                <span>
                  <Button
                    variant="default-outline" size="sm"
                    onClick={()=>this.setState({expanded: true})} >
                    <FontAwesomeIcon icon="plus"/>
                  </Button>
                  { _.map(this.props.value, (v, k) => {
                      return <span key={k} className="timeline-value-item">{k}: {v}</span>;
                  })}
                </span>
              }
            </div>
        );
    }
}

export default class TimelineRenderer extends React.Component {
    static propTypes = {
        name: PropTypes.string,
        params: PropTypes.string,
    }

    componentDidMount = () => {
        this.source = axios.CancelToken.source();
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

        return false;
    }

    state = {
        start_time: 0,
        table_start: 0,
        table_end: 0,
        loading: true,
        disabled: {},
        version: 0,
    };

    fetchRows = () => {
        let skip_components = [];
        _.map(this.state.disabled, (v,k)=>{
            if(v) {
                skip_components.push(k);
            };
        });

        let params = {
            type: "TIMELINE",
            timeline: this.props.name,
            start_time: this.state.start_time * 1000000,
            rows: 10,
            skip_components: skip_components,
        };

        let url = this.props.url || "v1/GetTable";

        this.source.cancel();
        this.source = axios.CancelToken.source();

        this.setState({loading: true});

        api.get(url, params, this.source.token).then((response) => {
            if (response.cancel) {
                return;
            }

            let pageData = PrepareData(response.data);
            this.setState({
                table_start: response.data.start_time / 1000000,
                table_end:  response.data.end_time / 1000000,
                columns: pageData.columns,
                rows: pageData.rows,
                version: Date(),
            });
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

    getTimelineClass = (name, timelines) => {
        for(let i=0;i<timelines.length;i++) {
            if (timelines[i].id === name) {
                return "timeline-item-" + (i + 1);
            };
        }
        return "";
    }

    render() {
        let params = JSON.parse(this.props.params);
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
        for (let i=0;i<params.timelines.length;i++) {
            let timeline = params.timelines[i];
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

        return <div className="super-timeline">Super-timeline {this.props.name}
                 <Timeline
                   groups={groups}
                   items={items}
                   defaultTimeStart={moment(smallest).add(-1, "day")}
                   defaultTimeEnd={moment(largest).add(1, "day")}
                   itemTouchSendsClick={true}
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
                 >
                   <TimelineMarkers>
                     <CustomMarker date={this.state.start_time} />
                   </TimelineMarkers>
                 </Timeline>
                 { this.state.columns &&
                   <VeloTable rows={this.state.rows}
                              columns={this.state.columns}
                              renderers={{
                                  "Time": (cell, row, rowIndex) => {
                                      return<div className={this.getTimelineClass(row._Source, params.timelines)}>
                                              <VeloTimestamp usec={cell / 1000000}/>
                                            </div>;
                                  },
                                  "Data": (cell, row, rowIndex) => {
                                      return <TimelineValueRenderer value={cell}/>;
                                  },
                              }}
                   />
                 }
               </div>;
    }
}
