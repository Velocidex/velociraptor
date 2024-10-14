
import React, { Component } from 'react';
import PropTypes from 'prop-types';

import Button from 'react-bootstrap/Button';
import Modal from 'react-bootstrap/Modal';
import T from '../i8n/i8n.jsx';
import moment from 'moment';
import 'moment-timezone';

import { runArtifact } from "../flows/utils.jsx";
import VeloTimestamp from "../utils/time.jsx";


export default class DeleteTimelineRanges extends Component {
    static propTypes = {
        client_id: PropTypes.string,
        start_time: PropTypes.object,
        end_time: PropTypes.object,
        onClose: PropTypes.func.isRequired,
        artifact: PropTypes.string,
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
        let start_time = moment(this.props.start_time).format();
        let end_time = moment(this.props.end_time).format();

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

                  <div>
                    <VeloTimestamp iso={start_time}/> -
                    <VeloTimestamp iso={end_time}/>
                  </div>
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
