import React, { Component } from 'react';
import PropTypes from 'prop-types';
import Modal from 'react-bootstrap/Modal';
import FlowLogs from '../flows/flow-logs.jsx';
import T from '../i8n/i8n.jsx';

class ViewCellLogsTable extends FlowLogs {
    static propTypes = {
        cell: PropTypes.object,
        notebook_metadata: PropTypes.object.isRequired,
    };

    state = {
        level_filter: "all",
        level_column: "Level",
    }

    getVersion = ()=>{}

    getParams = ()=>{
        return  {
            notebook_id: this.props.notebook_metadata &&
                this.props.notebook_metadata.notebook_id,
            cell_id: this.props.cell && this.props.cell.cell_id,
            cell_version: this.props.cell && this.props.cell.current_version,
            type: "logs",
        };
    }
}

export default class ViewCellLogs extends Component {
    static propTypes = {
        cell: PropTypes.object,
        notebook_metadata: PropTypes.object.isRequired,
        closeDialog: PropTypes.func.isRequired,
    };

    render() {
        return (
            <Modal show={true}
                   className="full-height"
                   dialogClassName="modal-90w"
                   enforceFocus={false}
                   scrollable={true}
                   onHide={this.props.closeDialog}>
              <Modal.Header closeButton>
                <Modal.Title>{T("View Cell Logs")}</Modal.Title>
              </Modal.Header>
              <Modal.Body>
                <ViewCellLogsTable
                  cell={this.props.cell}
                  notebook_metadata={this.props.notebook_metadata}
                />
              </Modal.Body>
              <Modal.Footer>
              </Modal.Footer>
            </Modal>
        );
    }
}
