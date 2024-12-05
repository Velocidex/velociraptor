import _ from 'lodash';

import PropTypes from 'prop-types';
import React, { Component } from 'react';
import Modal from 'react-bootstrap/Modal';
import T from '../i8n/i8n.jsx';
import {CancelToken} from 'axios';
import VeloValueRenderer from '../utils/value.jsx';
import Button from 'react-bootstrap/Button';
import { sprintf } from 'sprintf-js';
import api from '../core/api-service.jsx';
import Select from 'react-select';
import Col from 'react-bootstrap/Col';
import Form from 'react-bootstrap/Form';
import Row from 'react-bootstrap/Row';

const timestampRegex = /\d{4}-\d{2}-\d{2}[T ]\d{1,2}:\d{2}:\d{2}/;

import { ToStandardTime } from '../utils/time.jsx';

export default class AnnotateDialog extends Component {
    static propTypes = {
        row: PropTypes.object,
        onClose: PropTypes.func.isRequired,
    }

    componentDidMount = () => {
        this.source = CancelToken.source();
        this.fetchGlobalTimelines();
    }

    state = {
        collapsed: true,
        global_timelines: [],
        note: "",
        notebook_id: "",
        title: "",
        name: "",
        super_timeline: "",
        time_column: "",
        message_column: "",
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

    isTimestamp = x=>{
        if (timestampRegex.test(x)) {
            return true;
        }

        return false;
    }

    annotateRow = ()=>{
        let timestamp = "";
        if (this.state.time_column) {
            timestamp = this.props.row[this.state.time_column];
        }

        let ts = ToStandardTime(timestamp || "1980-01-01T00:00:00Z");
        let event = Object.assign({}, this.props.row);
        if (this.state.message_column) {
            event.Message = this.props.row[this.state.message_column];
        }
        api.post("v1/AnnotateTimeline", {
            notebook_id: this.state.notebook_id,
            super_timeline: this.state.super_timeline,
            timestamp: ts.getTime() * 1000000,
            note: this.state.note,
            event_json: JSON.stringify(event),
        }, this.source.token).then(response=>{
            this.props.onClose();
        });
    }

    render() {
        let clsname = this.state.collapsed ? "compact": "";

        let global_timeline_options = _.map(this.state.global_timelines, x=>{
            return {value: x.name,
                    label: sprintf(T("%s (notebook %s)"), x.name, x.title),
                    notebook_id: x.notebook_id,
                    isFixed: true,
                    color: "#00B8D9"};
        });

        let time_column_options = [];
        _.each(this.props.row, (v, k)=>{
            if(this.isTimestamp(v)) {
                time_column_options.push( {value: k, label: k, isFixed: true});
            };
        });

        let message_column_options = _.map(this.props.row, (v, k)=>{
            return {value: k, label: k, isFixed: true};
        });

        return <Modal show={true}
                      enforceFocus={true}
                      scrollable={false}
                      size="lg"
                      dialogClassName="modal-90w"
                      onHide={this.props.onClose}>
                 <Modal.Header closeButton>
                   <Modal.Title>{T("Annotate Row")} </Modal.Title>
                 </Modal.Header>
                 <Modal.Body>
                   <Form.Group as={Row}>
                     <Form.Label column sm="3">
                       {T("Global Timeline")}
                     </Form.Label>
                     <Col sm="8">
                       <Select
                         isClearable
                         className="labels"
                         classNamePrefix="velo"
                         options={global_timeline_options}
                         onChange={(e)=>this.setState({
                             super_timeline: e && e.value,
                             notebook_id: e && e.notebook_id,
                         })}
                         placeholder={T("Select timeline name from global notebook")}
                         spellCheck="false"
                       />

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
                         options={message_column_options}
                         onChange={(e)=>this.setState({
                             message_column: e && e.value})}
                         placeholder={T("Message Column")}
                         spellCheck="false"
                       />
                     </Col>
                   </Form.Group>

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
                   <Form.Group as={Row}>
                     <Form.Label column sm="3">{T("Extra Data")}</Form.Label>
                     <Col sm="8">
                       <div className={clsname}
                            onClick={()=>this.setState({
                                collapsed: !this.state.collapsed})}>
                         <VeloValueRenderer value={this.props.row}/>
                       </div>
                     </Col>
                   </Form.Group>

                 </Modal.Body>
                 <Modal.Footer>
                   <Button variant="secondary"
                           onClick={this.props.onClose}>
                     {T("Cancel")}
                   </Button>
                   <Button variant="primary"
                           disabled={!this.state.notebook_id || !this.state.note}
                           onClick={this.annotateRow}>
                     {T("Submit")}
                   </Button>
                 </Modal.Footer>
               </Modal>;
    }
}
