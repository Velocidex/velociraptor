import _ from 'lodash';
import "./event-timeline.css";

import React, { Component }  from 'react';
import PropTypes from 'prop-types';
import Timeline, {
    TimelineMarkers,
    CustomMarker,
} from 'react-calendar-timeline';
import moment from 'moment-timezone';
import qs from 'qs';
import axios from 'axios';
import { PrepareData } from '../core/table.js';
import api from '../core/api-service.js';
import BootstrapTable from 'react-bootstrap-table-next';
import VeloValueRenderer from '../utils/value.js';
import Dropdown from 'react-bootstrap/Dropdown';

import Button from 'react-bootstrap/Button';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';

// We want to work in UTC everywhere
moment.tz.setDefault("Etc/UTC");

const normalizeTimestamp = (value) => {
    if (_.isNumber(value) && value > 0) {
        if (value > 20000000000) {
            value /= 1000;
        }

        if (value > 20000000000) {
            value /= 1000;
        }

        return value * 1000;
    }
    return value;
};

class EventTableRenderer  extends Component {
    static propTypes = {
        renderers: PropTypes.object,
        columns: PropTypes.array,
        rows: PropTypes.array,
    }

    state = {
        download: false,
        toggles: {},
    }

    defaultFormatter = (cell, row, rowIndex) => {
        return <VeloValueRenderer value={cell}/>;
    }

    render() {
        if (!this.props.rows || !this.props.columns) {
            return <div></div>;
        }

        let rows = this.props.rows;
        let columns = [{dataField: '_id', hidden: true}];
        for(var i=0;i<this.props.columns.length;i++) {
            var name = this.props.columns[i];
            let definition ={ dataField: name, text: name};
            if (name == "_ts") {
                definition.text = "Server Time";
            }
            if (this.props.renderers && this.props.renderers[name]) {
                definition.formatter = this.props.renderers[name];
            } else {
                definition.formatter = this.defaultFormatter;
            }

            if (this.state.toggles[name]) {
                definition["hidden"] = true;
            }

            columns.push(definition);
        }

        // Add an id field for react ordering.
        for (var j=0; j<rows.length; j++) {
            rows[j]["_id"] = j;
        }

        return (
            <div className="velo-table timeline-table">
              <BootstrapTable
                hover
                condensed
                data={rows}
                columns={columns}
                keyField="_id"
                headerClasses="hidden-header"
                bodyClasses="fixed-table-body"
              />
            </div>
        );
    }
}

export default class EventTimelineViewer extends React.Component {
    static propTypes = {
        // Render the toolbar buttons in our parent component.
        toolbar: PropTypes.func,
        client_id: PropTypes.string,
        artifact: PropTypes.string,
        mode: PropTypes.string,
        renderers: PropTypes.object,
        column_types: PropTypes.array,
    };

    componentDidMount = () => {
        this.source = axios.CancelToken.source();
        this.ts_source = axios.CancelToken.source();
        this.fetchAvailableTimes();
    }

    componentDidUpdate(prevProps, prevState, snapshot) {
        if (!_.isEqual(prevState.start_time, this.state.start_time)) {
            this.fetchRows();
            return true;
        };

        if (!_.isEqual(prevState.table_start, this.state.table_start)) {
            return true;
        }

        if (!_.isEqual(prevProps.mode, this.props.mode)) {
            this.fetchRows();
            return true;
        };

        if (!_.isEqual(prevProps.artifact, this.props.artifact)) {
            this.fetchAvailableTimes();
            return true;
        };

        if (!_.isEqual(prevState.row_count, this.state.row_count)) {
            this.fetchRows();
            return true;
        };

        return false;
    }

    componentWillUnmount() {
        this.source.cancel();
        this.ts_source.cancel();
    }

    fetchAvailableTimes = () => {
        this.ts_source.cancel();
        this.ts_source = axios.CancelToken.source();

        let client_id = this.props.client_id || "server";

        api.post("v1/ListAvailableEventResults", {
            client_id: client_id,
            artifact: this.props.artifact,
        }, this.source.token).then(resp => {
            if (resp.cancel || !resp.data.logs) return;

            let av_t = resp.data.logs[0].row_timestamps;
            if (this.props.mode === "Logs") {
                av_t = resp.data.logs[0].log_timestamps;
            }

            if (av_t && av_t.length > 0) {
                let ts = av_t[av_t.length-1]*1000;
                this.setState({
                    start_time: ts,
                    table_start: ts,
                    end_time: ts + 60*60*24*1000,
                });
                this.centerPage();
            }
            this.setState({
                available_timestamps: resp.data.logs[0].row_timestamps || [],
                available_log_timestamps: resp.data.logs[0].log_timestamps || [],
            });

            this.fetchRows();
        });
    }

    fetchRows = () => {
        let url = this.props.url || "v1/GetTable";

        this.source.cancel();
        this.source = axios.CancelToken.source();

        this.setState({loading: true});

        // Re-render the toolbar each time we fetch a new row.
        this.props.toolbar(this.renderToolbar);

        let mode = "CLIENT_EVENT";
        if (this.props.mode === "Logs") {
            mode = "CLIENT_EVENT_LOGS";
        }

        let params = {
            client_id: this.props.client_id,
            artifact: this.props.artifact,
            type: mode,
            start_time: parseInt(this.state.start_time / 1000),
            end_time: 2000000000,
            rows: this.state.row_count || 10,
        };

        api.get(url, params, this.source.token).then((response) => {
            if (response.cancel) {
                return;
            }

            let pageData = PrepareData(response.data);

            // The first time is the _ts column of the first row
            let table_start = null;
            let table_end = null;

            let rows = pageData.rows;
            if (rows.length > 0) {
                table_start = normalizeTimestamp(rows[0]["_ts"]);
                table_end = normalizeTimestamp(rows[rows.length-1]["_ts"]);
            }

            let columns = ["_ts"];
            _.each(pageData.columns, x=>{
                if(x!=="_ts") {
                    columns.push(x);
                }
            });

            this.setState({
                table_start: table_start,
                table_end:  table_end,
                columns: columns,
                rows: pageData.rows,
            });
        });
    };

    pageSizeSelector = () => {
        let options = [10, 20, 50, 100];

        return <Dropdown
                 className="page-size-dropdown"

                 onSelect={(value)=>{
                     this.setState({row_count: value});
                 }}
                 >
                 <Dropdown.Toggle variant="default" id="row_count_selector">
                   {this.state.row_count}
                 </Dropdown.Toggle>

                 <Dropdown.Menu>
                   {_.map(options, (x, idx)=>{
                       return <Dropdown.Item
                                key={idx}
                                href="#" disabled={x===this.state.row_count}
                                eventKey={x}>{x}
                              </Dropdown.Item>;
                   })}
                 </Dropdown.Menu>
               </Dropdown>;
    }

    centerPage = () => {
        let page_size = this.state.visibleTimeEnd - this.state.visibleTimeStart;
        if (page_size === 0) {
            page_size = 60*60*24*1000;
        }
        this.setState({
            start_time: this.state.table_start,
            visibleTimeStart: this.state.table_start - page_size/2,
            visibleTimeEnd: this.state.table_start + page_size/2,
        });
    }

    // Jump to the previous page.
    prevPage = ()=>{
        this.setState({
            start_time: this.state.start_time - 60*60*24*1000,
        });
        this.fetchRows();
    }

    nextPage = ()=>{
        if (this.state.table_end > 0) {
            let page_size = this.state.visibleTimeEnd - this.state.visibleTimeStart;
            this.setState({
                start_time: this.state.table_end + 1000,
            });

            // Only scroll the timeline once we go past the view port.
            if (this.state.table_end > this.state.visibleTimeEnd) {
                this.setState({
                    visibleTimeStart: this.state.table_end + 1,
                    visibleTimeEnd: this.state.table_end + page_size,
                });
            }

            this.fetchRows();
        }
    }

    startDownload = (type) => {
        return true;
    }

    renderToolbar = () => {
        let mode = "CLIENT_EVENT";
        if (this.props.mode === "Logs") {
            mode = "CLIENT_EVENT_LOGS";
        }

        let start_time = moment(this.state.visibleTimeStart).format();
        let end_time = moment(this.state.visibleTimeEnd).format();
        let basename = `${this.props.artifact}-${start_time}-${end_time}-${this.props.client_id}`;
        let downloads_json = {
            columns: this.state.columns,
            client_id: this.props.client_id,
            artifact: this.props.artifact,
            type: mode,
            start_time: parseInt(this.state.visibleTimeStart / 1000),
            end_time: parseInt(this.state.visibleTimeEnd / 1000),
            rows: 1,
            download_format: "json",
            download_filename: basename,
        };

        let downloads_csv = Object.assign({}, downloads_json);
        downloads_csv.download_format = "csv";

        return <>
                 {this.pageSizeSelector()}
                 <Dropdown>
                   <Dropdown.Toggle variant="default">
                     <FontAwesomeIcon icon="download"/>
                   </Dropdown.Toggle>
                   <Dropdown.Menu>
                     <Dropdown.Item as="a"
                       href={api.base_path + "/api/v1/DownloadTable?" +
                             qs.stringify(downloads_csv,  {indices: false})}
                       variant="default" type="button">
                       <FontAwesomeIcon icon="file-csv"/>
                       <span className="button-label">
                         <div className="download-format">CSV</div>
                         <div className="download-time-range">
                           {start_time} - {end_time}
                         </div>
                       </span>
                     </Dropdown.Item>
                     <Dropdown.Item as="a"
                       href={api.base_path + "/api/v1/DownloadTable?" +
                             qs.stringify(downloads_json,  {indices: false})}
                       variant="default" type="button">
                       <FontAwesomeIcon icon="file-code"/>
                       <span className="button-label">
                         <div className="download-format">JSON</div>
                         <div className="download-time-range">
                           {start_time} - {end_time}
                         </div>
                       </span>
                     </Dropdown.Item>
                   </Dropdown.Menu>
                 </Dropdown>

                 <Button title="Previous"
                         onClick={() => this.prevPage()}
                         variant="default">
                   <FontAwesomeIcon icon="backward"/>
                 </Button>

                 <Button title="Center"
                         onClick={() => this.centerPage()}
                         variant="default">
                   <FontAwesomeIcon icon="crosshairs"/>
                 </Button>

                 <Button title="Next"
                         onClick={() => this.nextPage()}
                         variant="default">
                   <FontAwesomeIcon icon="forward"/>
                 </Button>
               </>;
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
        row_count: 10,
        visibleTimeStart: moment().startOf("day").valueOf(),
        visibleTimeEnd: moment().startOf("day").add(1, "day").valueOf(),

        available_timestamps: [],
        available_log_timestamps: [],
    }

    render() {
        let groups =  [
            {
                id: -1,
                title: "Table View",
            },
            {
                id: 1,
                title: "Available",
            },
            {
                id: 2,
                title: "Logs",
            },
        ];

        let items = [{
            id:-1, group: -1,
            start_time: moment(this.state.table_start),
            end_time: moment(this.state.table_end),
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


        let adder = (ts, group_id)=>{
            items.push({
                id: items.length, group: group_id,
                ts: ts,
                start_time: ts * 1000,
                end_time: (ts + 60*60*24)*1000,
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
            });
        };

        _.each(this.state.available_timestamps, x=>adder(x, 1));
        _.each(this.state.available_log_timestamps, x=>adder(x, 2));

        return <>
                 <Timeline
                   groups={groups}
                   items={items}
                   defaultTimeStart={moment().add(-1, "day")}
                   defaultTimeEnd={moment().add(1, "day")}
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
                   visibleTimeStart={this.state.visibleTimeStart}
                   visibleTimeEnd={this.state.visibleTimeEnd}
                   onTimeChange={this.handleTimeChange}
                 >
                   <TimelineMarkers>
                     <CustomMarker
                       date={this.state.start_time || new Date()} >
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
                 <EventTableRenderer
                   renderers={this.props.renderers}
                   rows={this.state.rows}
                   columns={this.state.columns}
                 />
               </>;
    }
};
