import "./timeline.css";

import React, { Component } from 'react';
import PropTypes from 'prop-types';
import _ from 'lodash';
import Timeline, {
    TimelineMarkers,
    CustomMarker,
    TimelineHeaders,
    SidebarHeader,
    DateHeader,
} from 'react-calendar-timeline';
import api from '../core/api-service.jsx';
import {CancelToken} from 'axios';
import { PrepareData } from '../core/table.jsx';
import VeloValueRenderer from '../utils/value.jsx';
import Form from 'react-bootstrap/Form';
import { JSONparse } from '../utils/json_parse.jsx';
import VeloTimestamp, {
    FormatRFC3339,
    ToStandardTime,
} from '../utils/time.jsx';

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
import Modal from 'react-bootstrap/Modal';
import Col from 'react-bootstrap/Col';
import Row from 'react-bootstrap/Row';
import { ColumnFilter, ColumnToggle } from '../core/paged-table.jsx';
import DateTimePicker from '../widgets/datetime.jsx';
import Dropdown from 'react-bootstrap/Dropdown';
import Alert from 'react-bootstrap/Alert';
import UserConfig from '../core/user.jsx';
import ColumnResizer from "../core/column-resizer.jsx";

// In ms
const TenYears =  10 * 365 * 24 * 60 * 60 * 1000;
const POLL_TIME = 2000;

const FixedColumns = {
    "Timestamp": 1,
    "Description": 1,
    "Message": 1,
};

const ms_to_ns = (t)=>{
    return (t || 0) * 1000000;
};

const ns_to_ms = (t)=>{
    return (t || 0) / 1000000;
};

const sec_to_ms = t=>{
    return (t || 0) * 1000;
}

class DeleteComponentDialog extends Component {
    static propTypes = {
        notebook_id: PropTypes.string,
        super_timeline: PropTypes.string,
        component: PropTypes.string,
        onClose: PropTypes.func,
    }

    componentDidMount = () => {
        this.source = CancelToken.source();
    }

    componentWillUnmount() {
        if (this.interval) {
            clearInterval(this.interval);
        }
        this.source.cancel();
    }

    removeTimeline = ()=>{
        api.post("v1/CollectArtifact", {
            client_id: "server",
            artifacts: ["Server.Utils.RemoveTimeline"],
            specs: [{artifact: "Server.Utils.RemoveTimeline",
                     parameters:{"env": [
                         {"key": "NotebookId", "value": this.props.notebook_id},
                         {"key": "Timeline", "value": this.props.super_timeline},
                         {"key": "ChildName", "value": this.props.component},
                     ]},
                    }],
        }, this.source.token).then(response=>{
            // Hold onto the flow id.
            this.setState({
                loading: true,
                lastOperationId: response.data.flow_id,
            });

            // Start polling for flow completion.
            this.interval = setInterval(() => {
                api.get("v1/GetFlowDetails", {
                    client_id: "server",
                    flow_id: this.state.lastOperationId,
                }, this.source.token).then((response) => {
                    let context = response.data.context;
                    if (context.state === "RUNNING") {
                        return;
                    }

                    // Done! Close the dialog.
                    clearInterval(this.interval);
                    this.interval = undefined;
                    this.props.onClose();
                    this.setState({loading: false});
                });
            }, POLL_TIME);
        });
    }

    render() {
        return <Modal show={true}
                      size="lg"
                      dialogClassName="modal-90w"
                      onHide={this.props.onClose}>
                 <Modal.Header closeButton>
                   <Modal.Title>{T("Remove Timeline")}</Modal.Title>
                 </Modal.Header>
                 <Modal.Body>
                   <Alert variant="warning">
                     <h1>{this.props.component}</h1>
                     {T("You are about to remove this timeline")}
                   </Alert>
                 </Modal.Body>
                 <Modal.Footer>
                   <Button variant="secondary" onClick={this.props.onClose}>
                     {T("Close")}
                   </Button>
                   <Button variant="primary" onClick={this.removeTimeline}>
                     {T("Yes do it!")}
                   </Button>
                 </Modal.Footer>
               </Modal>;

    }
}

class AnnotationDialog extends Component {
    static propTypes = {
        timestamp: PropTypes.number,
        event: PropTypes.object,
        notebook_id: PropTypes.string,
        super_timeline: PropTypes.string,
        onClose: PropTypes.func,
    }

    state = {
        note: "",
    }

    componentDidMount = () => {
        this.source = CancelToken.source();

        // If this is already an annotation, start with the previous
        // note.
        let event = this.props.event || {};
        this.setState({note: event.Notes});
    }

    componentWillUnmount() {
        this.source.cancel();
    }

    updateNote = ()=>{
        api.post("v1/AnnotateTimeline", {
            notebook_id: this.props.notebook_id,
            super_timeline: this.props.super_timeline,
            timestamp: this.props.timestamp,
            note: this.state.note,
            event_json: JSON.stringify(this.props.event),
        }, this.source.token).then(response=>{
            this.props.onClose();
        });
    }

    render() {
        return <Modal show={true}
                      size="lg"
                      dialogClassName="modal-90w"
                      onHide={this.props.onClose}>
                 <Modal.Header closeButton>
                   <Modal.Title>{T("Annotate Event")}</Modal.Title>
                 </Modal.Header>
                 <Modal.Body>
                   <VeloValueRenderer value={this.props.event}/>
                   <Form>
                     <Form.Group as={Row}>
                       <Form.Label column sm="3">{T("Note")}</Form.Label>
                       <Col sm="8">
                         <Form.Control as="textarea" rows={1}
                                       placeholder={T("Enter short annotation")}
                                       spellCheck="true"
                                       value={this.state.note || ""}
                                       onChange={e => this.setState({note: e.target.value})}
                         />
                         </Col>
                     </Form.Group>
                   </Form>
                 </Modal.Body>
                 <Modal.Footer>
                   <Button variant="secondary" onClick={this.props.onClose}>
                     {T("Close")}
                   </Button>
                   <Button variant="primary" onClick={this.updateNote}>
                     {T("Yes do it!")}
                   </Button>
                 </Modal.Footer>
               </Modal>;
    }
}


class TimelineTableRow extends Component {
    static propTypes = {
        row: PropTypes.object,
        columns: PropTypes.array,
        notebook_id: PropTypes.string,
        super_timeline: PropTypes.string,
        timeline_class: PropTypes.string,
        onUpdate: PropTypes.func,
        seekToTime: PropTypes.func,
        column_widths: PropTypes.object,
        setWidth: PropTypes.func,  // func(column, width)
    }

    state = {
        expanded: false,
        showAnnotateDialog: false,
    }

    componentDidMount = () => {
        this.source = CancelToken.source();
    }

    componentWillUnmount() {
        this.source.cancel();
    }

    // To delete the note we propagate the GUID and unset the time.
    deleteNote = ()=>{
        api.post("v1/AnnotateTimeline", {
            notebook_id: this.props.notebook_id,
            super_timeline: this.props.super_timeline,
            event_json: JSON.stringify({
                _AnnotationID: this.props.row._AnnotationID,
                _OriginalTime: this.props.row.Timestamp,
            }),
        }, this.source.token).then(response=>{
            this.props.onUpdate();
            this.setState({expanded: false});
        });
    }

    renderCell = (row, column, i)=>{
        let td = <></>;

        if(column=="Message") {
            td = <td>
                   <VeloValueRenderer value={row.Message}/>
                 </td>;
        } else if(column=="Notes") {
            td = <td>
                   <VeloValueRenderer value={row.Notes}/>
                 </td>;
        } else {
            td = <td>
                   <VeloValueRenderer value={row[column]}/>
                 </td>;
        }

        return  <React.Fragment key={i}>
                  {td}
                  <ColumnResizer
                    width={this.props.column_widths[column]}
                    setWidth={x=>this.props.setWidth(column, x)}
                  />
                </React.Fragment>;
    };

    render() {
        let event = this.props.row || {};
        let row_class = "timeline-data ";
        if(!this.state.expanded) {
            row_class += "hidden";
        }

        let timestamp = ToStandardTime(event.Timestamp).getTime() * 1000000;

        return (
            <React.Fragment >
              <tr className="row-selected"
                  onClick={e=>this.setState({expanded: !this.state.expanded})}
              >
                <td className={"timeline-group " + this.props.timeline_class}>
                </td>
                <td className="time">
                  <VeloTimestamp usec={event.Timestamp || ""}/>
                </td>
                {_.map(this.props.columns || [],
                       (x, i)=>this.renderCell(event, x, i))}
              </tr>
              <tr className={row_class}>
                <td className={"timeline-group " + this.props.timeline_class}>
                </td>
                <td colSpan="30">
                  <ButtonGroup>
                    <ToolTip tooltip={T("Recenter event")}>
                      <Button variant="default"
                              onClick={()=>{
                                  this.props.seekToTime(event.Timestamp);
                                  this.setState({expanded: false});
                              }}
                      >
                        <FontAwesomeIcon icon="crosshairs"/>
                      </Button>
                    </ToolTip>

                    { event._Source !== "Annotation" ?
                      <ToolTip tooltip={T("Annotate event")}>
                        <Button variant="default"
                                onClick={()=>this.setState(
                                    {showAnnotateDialog: true})}
                        >
                          <FontAwesomeIcon icon="note-sticky"/>
                        </Button>
                      </ToolTip>
                      : <>
                          <ToolTip tooltip={T("Update Annotation")}>
                            <Button variant="default"
                                    onClick={()=>this.setState(
                                        {showAnnotateDialog: true})}
                            >
                              <FontAwesomeIcon icon="edit"/>
                            </Button>
                          </ToolTip>
                          <ToolTip tooltip={T("Delete Annotation")}>
                            <Button variant="default"
                                    onClick={()=>this.deleteNote()}
                            >
                              <FontAwesomeIcon icon="trash"/>
                            </Button>
                          </ToolTip>
                        </>
                    }
                  </ButtonGroup>
                  <VeloValueRenderer value={event} />
                </td>
              </tr>
              { this.state.showAnnotateDialog &&
                <AnnotationDialog
                  timestamp={timestamp}
                  notebook_id={this.props.notebook_id}
                  super_timeline={this.props.super_timeline}
                  event={event}
                  onClose={() => {
                      this.setState({showAnnotateDialog: false});
                      if(this.props.onUpdate) {
                          this.props.onUpdate();
                      };
                  }}
                />}
            </React.Fragment>
        );
    }
}

const fixed_columns = ["Message", "Notes"];

class TimelineTableRenderer  extends Component {
    static propTypes = {
        rows: PropTypes.array,
        timelines: PropTypes.object,
        extra_columns: PropTypes.array,
        notebook_id: PropTypes.string,
        super_timeline: PropTypes.string,
        onUpdate: PropTypes.func,
        transform: PropTypes.object,
        setTransform: PropTypes.func,
        seekToTime: PropTypes.func,
    }

    state = {
        column_widths: {},
        columns: [],
        extra_columns: [],
    }

    componentDidMount = () => {
        this.setState({
            extra_columns: this.props.extra_columns,
            columns: fixed_columns.concat(this.props.extra_columns)
        });
    }

    componentDidUpdate = (prevProps, prevState, rootNode) => {
        if(!_.isEqual(this.props.extra_columns, this.state.extra_columns)) {
            let columns = fixed_columns.concat(this.props.extra_columns);
            this.setState({
                columns: this.mergeColumns(columns),
                extra_columns: this.props.extra_columns,
            });
            return true;
        }
        return false;
    }

    // Insert the to_col right before the from_col
    swapColumns = (from_col, to_col)=>{
        let new_columns = [];
        let from_seen = false;

        if (from_col === to_col) {
            return;
        }

        _.each(this.state.columns, x=>{
            if(x === to_col) {
                if (from_seen) {
                    new_columns.push(to_col);
                    new_columns.push(from_col);
                } else {
                    new_columns.push(from_col);
                    new_columns.push(to_col);
                }
            }

            if(x === from_col) {
                from_seen = true;
            }

            if(x !== from_col && x !== to_col) {
                new_columns.push(x);
            }
        });
        this.setState({columns: new_columns});
    }

    // Merge new columns into the current table state in such a way
    // that the existing column ordering will not be changed.
    mergeColumns = columns=>{
        let lookup = {};
        _.each(columns, x=>{
            lookup[x] = true;
        });
        let new_columns = [];
        let new_lookup = {};

        // Add the old columns only if they are also in the new set,
        // preserving their order.
        _.each(this.state.columns, c=>{
            if(lookup[c]) {
                new_columns.push(c);
                new_lookup[c] = true;
            }
        });

        // Add new columns if they were not already, preserving their
        // order.
        _.each(columns, c=>{
            if(!new_lookup[c])  {
                new_columns.push(c);
            }
        });

        return new_columns;
    }


    getTimelineClass = (name) => {
        if (name === "Annotation") {
            return "timeline-annotation";
        }

        let timelines = this.props.timelines.timelines;
        if (_.isArray(timelines)) {
            for(let i=0;i<timelines.length;i++) {
                if (timelines[i].id === name) {
                    return "timeline-item-" + ((i%7) + 1);
                };
            }
        }
        return "";
    }

    renderRow = (row, idx)=>{
        return (
            <TimelineTableRow
              key={idx}
              notebook_id={this.props.notebook_id}
              super_timeline={this.props.super_timeline}
              timeline_class={this.getTimelineClass(_.toString(row._Source))}
              row={row}
              columns={this.state.columns}
              seekToTime={this.props.seekToTime}
              onUpdate={this.props.onUpdate}
              column_widths={this.state.column_widths}
              setWidth={(column, width)=>{
                  let column_widths = Object.assign({}, this.state.column_widths);
                  column_widths[column] = width;
                  this.setState({column_widths: column_widths});
              }}
            />
        );
    }

    renderHeader = (column, idx)=>{
        let styles = {};
        let col_width = this.state.column_widths[column];
        if (col_width) {
            styles = {
                minWidth: col_width,
                maxWidth: col_width,
                width: col_width,
            };
        }

        let th = <></>;
        let classname = column==="Notes" ? "notes" : "";

        if(column==="Message") {
            th = (
                <th className="message"
                    style={styles}
                    onDragStart={e=>{
                        e.dataTransfer.setData("column", column);
                    }}
                    onDrop={e=>{
                        e.preventDefault();
                        this.swapColumns(
                            e.dataTransfer.getData("column"), column);
                    }}
                    onDragOver={e=>{
                        e.preventDefault();
                        e.dataTransfer.dropEffect = "move";
                    }}
                    draggable="true">
                  <span className="column-name">
                    { T("Message") }
                  </span>
                  <span className="sort-element">
                    <ButtonGroup>
                      <ColumnFilter column="message"
                                    transform={this.props.transform}
                                    setTransform={this.props.setTransform}
                      />
                    </ButtonGroup>
                  </span>
                </th>
            );

        } else {
            th = <th className={classname}
                     style={styles}
                     onDragStart={e=>{
                         e.dataTransfer.setData("column", column);
                     }}
                     onDrop={e=>{
                         e.preventDefault();
                         this.swapColumns(
                             e.dataTransfer.getData("column"), column);
                     }}
                     onDragOver={e=>{
                         e.preventDefault();
                         e.dataTransfer.dropEffect = "move";
                     }}
                     draggable="true">
                   { T(column) }
                 </th>;
        }

        return  <React.Fragment key={idx}>
                 { th }
                  <ColumnResizer
                    width={this.state.column_widths[column]}
                    setWidth={x=>{
                        let column_widths = Object.assign(
                            {}, this.state.column_widths);
                        column_widths[column] = x;
                        this.setState({column_widths: column_widths});
                    }}
                  />
                </React.Fragment>;
    }

    render() {
        return <Table className="paged-table">
                <thead>
                  <tr className="paged-table-header">
                    <th></th>
                    <th className="time">
                      { T("Timestamp") }
                    </th>
                    {_.map(this.state.columns || [], this.renderHeader)}
                  </tr>
                </thead>
                 <tbody className="fixed-table-body">
                   {_.map(this.props.rows, this.renderRow)}
                 </tbody>
               </Table>;
    }
}

class GroupRenderer extends Component {
    static propTypes = {
        notebook_id: PropTypes.string,
        super_timeline: PropTypes.string,
        group: PropTypes.object,
        setGroup: PropTypes.func,
        disabled: PropTypes.bool,
        deletable: PropTypes.bool,
        onUpdate: PropTypes.func,
    }

    toggle = ()=>{
        let group = Object.assign({}, this.props.group);
        group.disabled = !group.disabled;
        this.props.setGroup(group);
    }

    state = {
        showDeleteDialog: false,
    }

    render() {
        let group = this.props.group || {};
        let icon_class = "";
        if (this.props.disabled) {
            icon_class = "hidden_icon";
        }

        let del_class = "";
        if (!this.props.deletable) {
            del_class = "hidden_icon";
        }

        return (
            <ToolTip tooltip={group.title}>
              <ButtonGroup>
                <Button
                  className={del_class}
                  onClick={()=>this.setState({showDeleteDialog: true})}
                  variant="outline-default">
                  <FontAwesomeIcon icon="trash"/>
                </Button>
                <Button variant="outline-default"
                        onClick={this.toggle}>
                  <span className={icon_class}>
                    <FontAwesomeIcon icon={
                        ["far", !group.disabled ?
                         "square-check" : "square"]
                    }/>
                  </span>
                </Button>
                <Button variant="outline-default"
                        onClick={this.toggle}>
                  { group.title }
                </Button>
                {this.state.showDeleteDialog &&
                 <DeleteComponentDialog
                   notebook_id={this.props.notebook_id}
                   super_timeline={this.props.super_timeline}
                   component={group.title}
                   onClose={()=>{
                       this.setState({showDeleteDialog: false});
                       this.props.onUpdate();
                   }}
                 />}
              </ButtonGroup>
            </ToolTip>
        );
    }
}


export default class TimelineRenderer extends React.Component {
    static contextType = UserConfig;

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

        if (!_.isEqual(prevState.start_time_ms, this.state.start_time_ms)) {
            this.fetchRows();
            return true;
        };

        if (!_.isEqual(prevState.row_count, this.state.row_count)) {
            this.fetchRows();
            return true;
        };

        if (!_.isEqual(prevState.transform, this.state.transform)) {
            this.fetchRows();
            return true;
        }

        return false;
    }

    handleTimeChange = (visibleTimeStart, visibleTimeEnd) => {
        this.setState({
            visibleTimeStart: this.fromLocalTZ(visibleTimeStart),
            visibleTimeEnd: this.fromLocalTZ(visibleTimeEnd),
            scrolling: true
        });
    };

    state = {
        start_time_iso: "",
        start_time_ms: 0,
        table_start_ms: 0,
        table_end_ms: 0,
        loading: true,
        disabled: {},
        version: 0,
        row_count: 10,
        visibleTimeStart: 0,
        visibleTimeEnd: 0,
        toggles: {},
        transform: {},
        timelines: [],
    };

    setStartTime = ts_ms=>{
        let ts = new Date(ts_ms);
        let timezone = this.context.traits.timezone || "UTC";
        this.setState({
            start_time_ms: ts_ms,
            start_time_iso: FormatRFC3339(ts, timezone),
        });
    }

    fetchRows = (go_to_start_time_ms) => {
        let skip_components = [];
        _.map(this.state.disabled, (v,k)=>{
            if(v) {
                skip_components.push(k);
            };
        });

        let start_time_ms = this.state.start_time_ms || 0;
        if (go_to_start_time_ms) {
            start_time_ms = go_to_start_time_ms;
        }

        let transform = this.state.transform || {};

        let params = {
            type: "TIMELINE",
            timeline: this.props.name,
            start_time: ms_to_ns(start_time_ms),
            rows: this.state.row_count,
            skip_components: skip_components,
            notebook_id: this.props.notebook_id,
            filter_column: transform.filter_column,
            filter_regex: transform.filter_regex,
        };

        let url = "v1/GetTable";

        this.source.cancel();
        this.source = CancelToken.source();

        this.setState({loading: true});

        api.get(url, params, this.source.token).then((response) => {
            if (response.cancel) {
                return;
            }
            let start_time_ms = ns_to_ms(response.data.start_time);
            let pageData = PrepareData(response.data);
            let timelines = response.data.timelines;

            this.setState({
                table_start_ms: start_time_ms,
                table_end_ms:  ns_to_ms(response.data.end_time),
                columns: pageData.columns,
                rows: pageData.rows || [],
                version: Date(),
                timelines: timelines,
            });

            // If no rows, return after the setState to make sure to
            // clear the rows to force the table to clear.
            if (_.isEmpty(pageData.rows)) {
                return;
            }
            // If the visible table is outside the view port, adjust
            // the view port.
            if (this.state.visibleTimeStart === 0 ||
                start_time_ms > this.state.visibleTimeEnd ||
                start_time_ms < this.state.visibleTimeStart) {
                let diff = (this.state.visibleTimeEnd -
                            this.state.visibleTimeStart) || (60 * 60 * 10000);

                let visibleTimeStart = start_time_ms - diff * 0.1;
                let visibleTimeEnd = start_time_ms + diff * 0.9;
                this.setState({visibleTimeStart: visibleTimeStart,
                               visibleTimeEnd: visibleTimeEnd});

                this.setStartTime(start_time_ms);
            }

            this.updateToggles(pageData.rows);
        });
    };

    groupRenderer = ({ group }) => {
        return <GroupRenderer
                 setGroup={group=>{
                     let disabled = this.state.disabled;
                     disabled[group.id] = group.disabled;
                     this.setState({disabled: disabled});
                     this.fetchRows();
                 }}
                 notebook_id={this.props.notebook_id}
                 super_timeline={this.props.name}
                 group={group}
                 disabled={group.id === -1}
                 deletable={group.id !== -1 && group.id !== "Annotation"}
                 onUpdate={this.fetchRows}
               />;
    };

    nextPage = ()=>{
        if (this.state.table_end_ms > 0) {
            this.setStartTime(this.state.table_end_ms + 1);
        }
    }

    updateToggles = rows=>{
        // Find all unique columns
        let _columns={};
        let columns = [];
        let toggles = {...this.state.toggles};

        _.each(this.state.rows, row=>{
            _.each(row, (v, k)=>{
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

    lastEvent = ()=>{
        let timelines = this.state.timelines || [];
        let last_event = 0;
        for(let i=0;i<timelines.length;i++) {
            if(last_event < timelines[i].end_time_ms) {
                last_event = timelines[i].end_time_ms;
            }
        }
        return last_event * 1000;
    }

    toLocalTZ = ts=>{
        let timezone = this.context.traits.timezone || "UTC";
        let zone = moment.tz.zone(timezone);
        if (!zone) {
            return ts;
        }
        return moment.utc(ts).subtract(zone.utcOffset(ts), "minutes").valueOf();
    }

    fromLocalTZ = ts=>{
        let timezone = this.context.traits.timezone || "UTC";
        let zone = moment.tz.zone(timezone);
        if (!zone) {
            return ts;
        }
        return moment.utc(ts).add(zone.utcOffset(ts), "minutes").valueOf();
    }

    renderSidebarHeader = getRootProps=>{
        let icon_class = "";
        let icon = "";
        let timelines = this.state.timelines || [];
        let enabled = _.filter(timelines, x=>x.active);
        let all_disabled = enabled.length === 0;
        let all_enabled = enabled.length === timelines.length;

        if (all_enabled) {
            icon = "square-check";
        } else if(all_disabled) {
            icon = "square";
        } else {
            icon = "square-minus";
        }

        return <div {...getRootProps()}>
                 <ButtonGroup className="timeline-sidebar-buttons">
                   <Button
                     className="hidden_icon"
                     variant="outline-default">
                   </Button>
                   <Button variant="outline-default"
                           onClick={()=>{
                               // Clear all the disabled timelines
                               // (i.e. show them all).
                               let disabled = {};

                               // Unless they are all enabled, in
                               // which case we disable them all.
                               if(all_enabled) {
                                   _.each(this.state.timelines, x=>{
                                       disabled[x.id] = true;
                                   });
                               }
                               this.setState({disabled: disabled});
                               setTimeout(this.fetchRows, 100);
                           }}>
                     <span className={icon_class}>
                       <FontAwesomeIcon icon={["far", icon]}/>
                     </span>
                   </Button>
                 </ButtonGroup>
               </div>;
    }

    render() {
        let super_timeline = {timelines: this.state.timelines || []};
        if(_.isEmpty(super_timeline.timelines)) {
            if (_.isString(this.props.params)) {
                super_timeline = JSONparse(this.props.params);
                if(!super_timeline) {
                    return <></>;
                }
            } else if(_.isObject(this.props.params)) {
                super_timeline = this.props.params;
            }
        }

        // Special groups must come first.
        let groups = [{id: -1, title: "Table View"},
                      {id: "Annotation", title: "Annotation",
                       disabled: this.state.disabled.Annotation}];
        let items = [{
            id:-1, group: -1,
            start_time: this.toLocalTZ(this.state.table_start_ms),
            end_time: this.toLocalTZ(this.state.table_end_ms),
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
            let start_ms = sec_to_ms(timeline.start_time);
            let end_ms = sec_to_ms(timeline.end_time);
            if (start_ms < smallest) {
                smallest = start_ms;
            }

            if (end_ms > largest) {
                largest = end_ms;
            }

            // Handle the annotation timeline specifically
            if (timeline.id === "Annotation") {
                items.push({
                    id: i+1, group: timeline.id,
                    start_time: this.toLocalTZ(start_ms),
                    end_time: this.toLocalTZ(end_ms),
                    canMove: false,
                    canResize: false,
                    canChangeGroup: false,
                    itemProps: {
                        className: 'timeline-annotation',
                        style: {
                            background: undefined,
                            color: undefined,
                        }
                    },
                });

            } else {
                groups.push({
                    id: timeline.id,
                    disabled: this.state.disabled[timeline.id],
                    title: timeline.id,
                });

                items.push({
                    id: i+1, group: timeline.id,
                    start_time: this.toLocalTZ(start_ms),
                    end_time: this.toLocalTZ(end_ms),
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
        }

        if (smallest > largest) {
            smallest = largest;
        }

        if (_.isNaN(smallest) || smallest < 0) {
            smallest = 0;
            largest = 0;
        }

        if (largest - smallest > TenYears) {
            largest = smallest + TenYears;
        }

        let extra_columns = [];
        _.each(this.state.toggles, (v,k)=>{
            if(!v) { extra_columns.push(k); }});

        let page_sizes = _.map([10, 25, 30, 50, 100], x=>{
            return <Dropdown.Item
                     as={Button}
                     variant="default"
                     key={x}
                     active={x===this.state.row_count}
                     onClick={()=>this.setState({row_count: x})} >
                     { x }
                   </Dropdown.Item>;
        });

        return <div className="super-timeline">Super-timeline {this.props.name}
                 <Navbar className="toolbar">
                   <ButtonGroup>
                     { this.renderColumnSelector() }
                     <ToolTip tooltip={T("Go to First Event")}>
                       <Button title="Start"
                               onClick={()=>{
                                   this.fetchRows(1);
                               }}
                               variant="default">
                         <FontAwesomeIcon icon="backward-fast"/>
                       </Button>
                     </ToolTip>

                     <ToolTip tooltip={T("Page Size")}>
                       <Dropdown as={ButtonGroup} >
                         <Dropdown.Toggle variant="default" id="dropdown-basic">
                           {this.state.row_count || 0}
                         </Dropdown.Toggle>

                         <Dropdown.Menu>
                           { page_sizes }
                         </Dropdown.Menu>
                       </Dropdown>
                     </ToolTip>
                     <DateTimePicker
                       value={this.state.start_time_iso}
                       onChange={value=>{
                           let ts = ToStandardTime(value);
                           if (_.isDate(ts)) {
                               let time = ts.getTime();
                               this.fetchRows(time);
                               this.setStartTime(time);
                           }
                       }}/>
                     <ToolTip tooltip={T("Next Page")}>
                       <Button title="Next"
                               onClick={() => {this.nextPage(); }}
                               variant="default">
                         <FontAwesomeIcon icon="forward"/>
                       </Button>
                     </ToolTip>
                     <ToolTip tooltip={T("Last Event")}>
                       <Button title="Last"
                               onClick={() => {this.fetchRows(
                                   this.lastEvent() - 1000); }}
                               variant="default">
                         <FontAwesomeIcon icon="forward-fast"/>
                       </Button>
                     </ToolTip>

                   </ButtonGroup>
                 </Navbar>
                 <Timeline
                   groups={groups}
                   items={items}
                   defaultTimeStart={moment(this.toLocalTZ(smallest)).add(-1, "day")}
                   defaultTimeEnd={moment(this.toLocalTZ(largest)).add(1, "day")}
                   itemTouchSendsClick={true}
                   minZoom={60*1000}
                   dragSnap={1000}
                   traditionalZoom={true}
                   onCanvasClick={(groupId, time, e) => {
                       this.setStartTime(this.fromLocalTZ(time));
                   }}
                   onItemSelect={(itemId, e, time) => {
                       this.setStartTime(this.fromLocalTZ(time));
                       return false;
                   }}
                   onItemClick={(itemId, e, time) => {
                       this.setStartTime(this.fromLocalTZ(time));
                       return false;
                   }}
                   groupRenderer={this.groupRenderer}
                   onTimeChange={this.handleTimeChange}
                   visibleTimeStart={this.toLocalTZ(this.state.visibleTimeStart)}
                   visibleTimeEnd={this.toLocalTZ(this.state.visibleTimeEnd)}
                   sidebarWidth={200}
                 >
                   <TimelineHeaders>
                     <SidebarHeader>
                       {({ getRootProps }) => {
                           return this.renderSidebarHeader(getRootProps);
                       }}
                     </SidebarHeader>
                     <DateHeader unit="primaryHeader" />
                     <DateHeader />
                   </TimelineHeaders>
                   <TimelineMarkers>
                     <CustomMarker
                       date={this.toLocalTZ(this.state.start_time_ms)} >
                       { ({ styles, date }) => {
                           styles.backgroundColor = undefined;
                           styles.width = undefined;
                           styles.left = styles.left || 0;
                           return <div style={styles}
                                       className="timeline-marker"
                                  />;
                       }}
                     </CustomMarker>
                   </TimelineMarkers>
                 </Timeline>
                 { this.state.columns &&
                   <TimelineTableRenderer
                     super_timeline={this.props.name}
                     notebook_id={this.props.notebook_id}
                     timelines={super_timeline}
                     extra_columns={extra_columns}
                     onUpdate={this.fetchRows}
                     transform={this.state.transform}
                     setTransform={x=>this.setState({transform:x})}
                     seekToTime={t=>{
                         let time = ToStandardTime(this.fromLocalTZ(t));
                         if (_.isDate(time)) {
                             this.setStartTime(time.getTime());
                             this.fetchRows(time.getTime());
                         }
                     }}
                     rows={this.state.rows} />
                 }
               </div>;
    }
}
