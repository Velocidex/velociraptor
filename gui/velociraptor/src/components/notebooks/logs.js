import React, { Component } from 'react';
import PropTypes from 'prop-types';
import axios from 'axios';
import Modal from 'react-bootstrap/Modal';
import VeloPagedTable from '../core/paged-table.js';
import VeloTimestamp from "../utils/time.js";

export default class ViewCellLogs extends Component {
    static propTypes = {
        cell: PropTypes.object,
        notebook_metadata: PropTypes.object.isRequired,
        closeDialog: PropTypes.func.isRequired,
    };

    // Create an artifact template from the VQL
    componentDidMount = () => {
        this.source = axios.CancelToken.source();
    }

    componentWillUnmount() {
        this.source.cancel("unmounted");
    }

    render() {
        let params = {
            notebook_id: this.props.notebook_metadata &&
                this.props.notebook_metadata.notebook_id,
            cell_id: this.props.cell && this.props.cell.cell_id,
            type: "logs",
        };
        console.log(this.props.cell, this.props.notebook_metadata);
        return (
            <Modal show={true}
                   className="full-height"
                   dialogClassName="modal-90w"
                   enforceFocus={false}
                   scrollable={true}
                   onHide={this.props.closeDialog}>
              <Modal.Header closeButton>
                <Modal.Title>View Cell Logs</Modal.Title>
              </Modal.Header>
              <Modal.Body>
                <VeloPagedTable
                  className="col-12"
                  renderers={{
                      Timestamp: (cell, row, rowIndex) => {
                          return (
                              <VeloTimestamp usec={cell / 1000}/>
                          );
                      }
                  }}
                  params={params}
                />
              </Modal.Body>
              <Modal.Footer>
              </Modal.Footer>
            </Modal>
        );
    }
}
