import "./timelines.css";

import React from 'react';
import PropTypes from 'prop-types';
import _ from 'lodash';
import {CancelToken} from 'axios';
import api from '../core/api-service.jsx';

import parseHTML from '../core/sanitize.jsx';

import Modal from 'react-bootstrap/Modal';
import CreatableSelect from 'react-select/creatable';
import Select from 'react-select';
import Button from 'react-bootstrap/Button';
import Form from 'react-bootstrap/Form';
import Col from 'react-bootstrap/Col';
import Row from 'react-bootstrap/Row';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { JSONparse } from '../utils/json_parse.jsx';
import ToolTip from '../widgets/tooltip.jsx';
import { sprintf } from 'sprintf-js';

import T from '../i8n/i8n.jsx';

const POLL_TIME = 2000;

const timestampRegex = /\d{4}-\d{2}-\d{2}[T ]\d{1,2}:\d{2}:\d{2}/;

// Adds a new timeline cell below this one.
export class AddTimelineDialog extends React.Component {
    static propTypes = {
        notebook_metadata: PropTypes.object.isRequired,
        addCell: PropTypes.func,
        closeDialog: PropTypes.func.isRequired,
    }

    state = {
        timeline: "",
    }

    addTimeline = ()=>{
        if (this.state.timeline) {
            // Inject the timeline name in the scope so we dont have
            // to escape it for the template.
            this.props.addCell(
                '{{ Scope "Timeline" | Timeline }}', "Markdown",
                [{key: "Timeline", value:this.state.timeline}]);
        }
        this.props.closeDialog();
    }

    render() {
        let timelines = this.props.notebook_metadata &&
            this.props.notebook_metadata.timelines;
        let options = _.map(timelines, x=>{
            return {value: x, label: x, isFixed: true, color: "#00B8D9"};
        });
        return (
            <Modal show={true}
                   size="sm"
                   onHide={this.props.closeDialog} >
              <Modal.Header closeButton>
                <Modal.Title>Add Timeline</Modal.Title>
              </Modal.Header>
              <Modal.Body>
                <CreatableSelect
                  options={options}
                  classNamePrefix="velo"
                  onChange={(e)=>{
                      this.setState({timeline: e.value});
                  }}
                  placeholder={T("Timeline name")}
                  spellCheck="false"
                />

              </Modal.Body>
              <Modal.Footer>
                <Button variant="secondary"
                        onClick={this.props.closeDialog}>
                  Cancel
                </Button>
                <Button variant="primary"
                        onClick={this.addTimeline}>
                  Submit
                </Button>
              </Modal.Footer>
            </Modal>
        );
    }
}

// Adds the current VQL cell to a timeline
export class AddVQLCellToTimeline extends React.Component {
    static propTypes = {
        cell: PropTypes.object,
        notebook_metadata: PropTypes.object.isRequired,
        closeDialog: PropTypes.func.isRequired,
    }

    state = {
        // Super timeline - Can be created
        timeline: "",
        time_column: "",
        message_column: "",

        // name for new child timeline
        name: "",

        loading: false,

        global_timelines: [],
    }

    componentDidMount = () => {
        this.source = CancelToken.source();
        this.getTables();
        this.fetchGlobalTimelines();
    }

    componentWillUnmount() {
        if (this.interval) {
            clearInterval(this.interval);
        }
        this.source.cancel();
    }

    isTimestamp = x=>{
        if (timestampRegex.test(x)) {
            return true;
        }

        return false;
    }

    fetchGlobalTimelines = ()=>{
        api.get("v1/GetNotebooks", {
            include_timelines: true,
            count: 100,
            offset: 0,
        }, this.source.token).then(response=>{
            if (response.cancel) {
                return;
            }

            if(response && response.data && response.data.items) {
                let timelines = [];
                _.each(response.data.items, x=>{
                    _.each(x.timelines, y=>{
                        timelines.push({notebook_id: x.notebook_id,
                                        title: x.name,
                                        name: y});
                    });
                });
                this.setState({global_timelines: timelines});
            }
        });
    }

    getTables = ()=>{
        let tags = [];

        parseHTML(this.props.cell.output, {
            replace: (domNode) => {
                if (domNode.name === "velo-csv-viewer") {
                    let value = JSONparse(
                        decodeURIComponent(domNode.attribs.params), "");
                    tags.push(value);
                };
                return domNode;
            }
        });
        if (_.isEmpty(tags)) {
            return;
        }

        // Just get the first row from the first table. We look at all
        // the values to figure out which ones look like timestamps.
        let params = tags[0];
        params.rows = 1;
        api.get("v1/GetTable", params, this.source.token).then((response) => {
            if (response.cancel) {
                return;
            }

            if(response && response.data && response.data.columns) {
                // Filter all columns that look like a time
                let time_columns = [];
                let all_columns = [];
                let rows = response.data.rows;
                if (_.isEmpty(rows)) {
                    return;
                }
                let row = JSONparse(rows[0].json) || [];
                _.each(row, (x, idx)=>{
                    if (this.isTimestamp(x)) {
                        time_columns.push(response.data.columns[idx]);
                    };
                    all_columns.push(response.data.columns[idx]);
                });

                this.setState({time_columns: time_columns,
                               all_columns: all_columns});
            }
        });
    };

    addTimeline = ()=>{
        let env = {};
        let requests = this.props.notebook_metadata &&
            this.props.notebook_metadata.requests;

        // Replicate the notebook env and pass it to the artifact.
        // This is needed to run the cell query inside the artifact.
        _.each(requests, x=>{
            _.each(x.env, x=>{
                env[x.key] = x.value;
            });
        });

        api.post("v1/CollectArtifact", {
            client_id: "server",
            artifacts: ["Server.Utils.AddTimeline"],
            specs: [{artifact: "Server.Utils.AddTimeline",
                     parameters:{"env": [
                         {"key": "NotebookId", "value": this.state.timeline_notebook_id},
                         {"key": "Timeline", "value": this.state.timeline},
                         {"key": "ChildName", "value": this.state.name},
                         {"key": "Key", "value": this.state.time_column},
                         {"key": "MessageColumn",
                          "value": this.state.message_column},
                         {"key": "Query", "value": this.props.cell.input},
                         {"key": "RemoveLimit", "value": "Y"},
                         {"key": "Env", "value": JSON.stringify(env)},
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
                    this.props.closeDialog();
                    this.setState({loading: false});
                });
            }, POLL_TIME);

        });
    }

    render() {
        let global_timeline_options = _.map(this.state.global_timelines, x=>{
            return {value: x.name,
                    label: sprintf(T("%s (notebook %s)"), x.name, x.title),
                    notebook_id: x.notebook_id,
                    isFixed: true,
                    color: "#00B8D9"};
        });

        let all_column_options = _.map(this.state.all_columns, x=>{
            return {value: x, label: x, isFixed: true, color: "#00B8D9"};
        });
        let time_column_options = _.map(this.state.time_columns, x=>{
            return {value: x, label: x, isFixed: true, color: "#00B8D9"};
        });

        return (
            <Modal show={true}
                   size="lg"
                   onHide={this.props.closeDialog} >
              <Modal.Header closeButton>
                <Modal.Title>{T("Add Timeline")}</Modal.Title>
              </Modal.Header>
              <Modal.Body>

                <Form.Group as={Row}>
                  <ToolTip tooltip={T("Select timeline type")}>
                    <Form.Label column sm="3">{ T("Super Timeline")}</Form.Label>
                  </ToolTip>
                  <Col sm="8">
                     <Select
                       isClearable
                       className="labels"
                       classNamePrefix="velo"
                       options={global_timeline_options}
                       onChange={(e)=>this.setState({
                           timeline: e && e.value,
                           timeline_notebook_id: e && e.notebook_id,
                       })}
                       placeholder={T("Select timeline name from global notebook")}
                       spellCheck="false"
                     />
                  </Col>
                </Form.Group>

                <Form.Group as={Row}>
                  <Form.Label column sm="3">{T("Timeline Name")}</Form.Label>
                  <Col sm="8">
                    <Form.Control as="textarea"
                                  rows={1}
                                  placeholder={T("Child timeline name")}
                                  spellCheck="false"
                                  value={this.state.name}
                                  onChange={(e) => this.setState(
                                      {name: e.currentTarget.value})} />
                  </Col>
                </Form.Group>
                <Form.Group as={Row}>
                  <Form.Label column sm="3">{T("Time column")}</Form.Label>
                  <Col sm="8">
                    <Select
                      className="time-column"
                      classNamePrefix="velo"
                      options={time_column_options}
                      onChange={(e)=>this.setState({time_column: e && e.value})}
                      placeholder={T("Time Column")}
                      spellCheck="false"
                    />
                  </Col>
                </Form.Group>

                <Form.Group as={Row}>
                  <Form.Label column sm="3">{T("Message column")}</Form.Label>
                  <Col sm="8">
                    <Select
                      className="time-column"
                      classNamePrefix="velo"
                      options={all_column_options}
                      onChange={(e)=>this.setState({message_column: e && e.value})}
                      placeholder={T("Message Column")}
                      spellCheck="false"
                    />
                  </Col>
                </Form.Group>

              </Modal.Body>
              <Modal.Footer>
                <Button variant="secondary"
                        onClick={this.props.closeDialog}>
                  Cancel
                </Button>
                <Button variant="primary"
                        disabled={!this.state.timeline || !this.state.time_column ||
                                  !this.state.name || !this.state.time_column ||
                                  this.state.loading }
                        onClick={this.addTimeline}>
                  { this.state.loading ?  <><FontAwesomeIcon icon="spinner" spin /> {T("Running")} </>:
                    <> {T("Add Timeline")} </> }
                </Button>
              </Modal.Footer>
            </Modal>
        );
    }
}
