import _ from 'lodash';
import React from 'react';
import PropTypes from 'prop-types';
import T from '../i8n/i8n.jsx';
import Modal from 'react-bootstrap/Modal';
import Button from 'react-bootstrap/Button';
import Spinner from '../utils/spinner.jsx';
import BootstrapTable from 'react-bootstrap-table-next';
import { formatColumns } from "../core/table.jsx";
import ArtifactLink from '../artifacts/artifacts-link.jsx';

import {CancelToken} from 'axios';
import api from '../core/api-service.jsx';
import { runArtifact } from "./utils.jsx";

export default class AddFlowToHuntDialog extends React.Component {
    static propTypes = {
        client: PropTypes.object,
        flow: PropTypes.object,
        onClose: PropTypes.func.isRequired,
    };

    state = {
        loading: true,
        hunts: [],
        selected_hunt_id: null,
    }

    componentDidMount = () => {
        this.source = CancelToken.source();
        this.fetchCompatibleHunts();
    }

    componentWillUnmount() {
        this.source.cancel();
    }

    fetchCompatibleHunts = () => {
        let artifacts = this.props.flow && this.props.flow.request &&
            this.props.flow.request.artifacts;

        api.get("v1/ListHunts", {
            count: 2000,
            offset: 0,
        }, this.source.token).then((response) => {
            if (response.cancel) return;

            let hunts = response.data.items || [];
            let filtered_hunts = _.filter(hunts, x=>{
                return !_.isEmpty(_.intersection(artifacts, x.artifacts));
            });
            this.setState({
                hunts: filtered_hunts,
                loading: false,
            });
        });
    }

    addFlowToHunt = ()=>{
        let client_id = this.props.client && this.props.client.client_id;
        let flow_id = this.props.flow && this.props.flow.session_id;

        if (flow_id && client_id) {
            this.setState({loading: true});
            runArtifact("server",
                        "Server.Hunts.AddFlow",
                        {
                            FlowId: flow_id,
                            ClientId: client_id,
                            HuntId: this.state.selected_hunt_id,
                        }, ()=>{
                             this.props.onClose();
                            this.setState({loading: false});
                        }, this.source.token);
        }
    }

    render() {
        let artifacts = this.props.flow && this.props.flow.request &&
            this.props.flow.request.artifacts;

        const selectRow = {
            mode: "radio",
            clickToSelect: true,
            hideSelectColumn: true,
            classes: "row-selected",
            onSelect: row=>this.setState({selected_hunt_id: row.hunt_id}),
            selected: [],
        };

        if (!_.isEmpty(this.state.selected_hunt_id)) {
            selectRow.selected.push(this.state.selected_hunt_id);
        }

        let columns = formatColumns([
            {dataField: "hunt_id", text: T("HuntId")},
            {dataField: "hunt_description", text: T("Description")},
            {dataField: "create_time", text: T("Created"),
             type: "timestamp", sort: true},
        ]);

        return (
            <Modal show={true} className="max-height"
                   dialogClassName="modal-90w"
                   onHide={this.props.onClose}>
              <Modal.Header closeButton>
                <Modal.Title>{T("Manually add collection to hunt")}</Modal.Title>
              </Modal.Header>
              <Modal.Body><Spinner loading={this.state.loading} />
                { _.isEmpty(this.state.hunts) ?
                  <>
                    <h3>{T("No compatible hunts.")}</h3>
                    <p>
                      {T("Please create a hunt that collects one or more of the following artifacts.")}
                    </p>
                    <ul className="list-group">
                      {_.map(artifacts, (x, idx)=>{
                          return <li className="list-group-item" key={idx}>
                                   <ArtifactLink artifact={x} />
                                 </li>;
                      })}
                    </ul>
                  </>
                  :
                  <BootstrapTable
                    hover
                    condensed
                    ref={ n => this.node = n }
                    keyField="hunt_id"
                    bootstrap4
                    headerClasses="alert alert-secondary"
                    bodyClasses="fixed-table-body selectable"
                    data={this.state.hunts}
                    selectRow={ selectRow }
                    columns={columns}
                  />
                }
              </Modal.Body>
              <Modal.Footer>
                <Button variant="secondary" onClick={this.props.onClose}>
                  {T("Close")}
                </Button>
                <Button variant="primary"
                        disabled={_.isEmpty(this.state.selected_hunt_id)}
                        onClick={this.addFlowToHunt}>
                  {T("Yes do it!")}
                </Button>
              </Modal.Footer>
            </Modal>
        );
    }
};
