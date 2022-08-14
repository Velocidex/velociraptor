
import React, { Component } from 'react';
import PropTypes from 'prop-types';

import Button from 'react-bootstrap/Button';
import Modal from 'react-bootstrap/Modal';
import T from '../i8n/i8n.js';
import moment from 'moment-timezone';

import { runArtifact } from "../flows/utils.js";
import VeloTimestamp from "../utils/time.js";


export default class DeleteTimelineRanges extends Component {
    static propTypes = {
        client_id: PropTypes.string,
        start_time: PropTypes.number,
        end_time: PropTypes.number,
        onClose: PropTypes.func.isRequired,
    }

    deleteData = () => {
        let start_time = moment(this.props.start_time).format();
        let end_time = moment(this.props.end_time).format();

        runArtifact("server",
                    "Server.Utils.DeleteEvents",
                    {
                        Artifact: this.props.artifact,
                        ClientId: this.props.client_id,
                        StartTime: start_time,
                        EndTime: end_time,
                        ReallyDoIt: "Y",
                    }, ()=>{
                        // When we are done we can close the window.
                        this.props.onClose();
                    });
    }

    render() {
        return (
            <>
              <Modal show={true}
                     onHide={this.props.onClose}>
                <Modal.Header closeButton>
                  <Modal.Title>
                    {T("Delete Events")}
                  </Modal.Title>
                </Modal.Header>
                <Modal.Body>
                  <h1>{T("Delete Time Range")}</h1>

                  {T("Are you sure you want to delete all logs within the time range?")}

                  <VeloTimestamp iso={this.props.start_time}/> -
                  <VeloTimestamp iso={this.props.end_time}/>

                </Modal.Body>

                <Modal.Footer>
                  <Button variant="secondary"
                          onClick={this.props.onClose}>
                    {T("Close")}
                  </Button>
                  <Button variant="primary"
                          onClick={this.deleteData}>
                    {T("Kill it!")}
                  </Button>
                </Modal.Footer>
              </Modal>
            </>
        );
    }
}
