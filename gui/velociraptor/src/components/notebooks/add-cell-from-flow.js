import React from 'react';
import PropTypes from 'prop-types';

import api from '../core/api-service.js';
import Modal from 'react-bootstrap/Modal';
import BootstrapTable from 'react-bootstrap-table-next';
import filterFactory from 'react-bootstrap-table2-filter';
import Button from 'react-bootstrap/Button';
import VeloClientSearch from '../clients/search.js';

import { getClientColumns } from '../clients/clients-list.js';
import { getFlowColumns } from '../flows/flows-list.js';


export default class AddCellFromFlowDialog extends React.Component {
    static propTypes = {
        closeDialog: PropTypes.func.isRequired,
        addCell: PropTypes.func,
    }


    componentDidMount = () => {
        this.setSearch("all");
    }

    setSearch = (query) => {
        api.get('/v1/SearchClients', {
            query: query,
            count: 50,
        }).then(resp => {
            let items = resp.data && resp.data.items;
            items = items || [];
            this.setState({loading: false, clients: items});
        });
    }

    addCellFromFlow = (flow) => {
        let client_id = flow.client_id;
        var flow_id = flow.session_id;
        var query = "SELECT * \nFROM source(\n";
        var sources = flow["artifacts_with_results"] || flow["request"]["artifacts"];
        query += "    artifact='" + sources[0] + "',\n";
        for (var i=1; i<sources.length; i++) {
            query += "    -- artifact='" + sources[i] + "',\n";
        }
        query += "    client_id='" + client_id + "', flow_id='" +
            flow_id + "')\nLIMIT 50\n";

        this.props.addCell(query, "VQL");
        this.props.closeDialog();
    }

    fetchFlows = (selectedClient) => {
        let client_id = selectedClient.client_id;
        api.get("v1/GetClientFlows/" + client_id, {
            count: 100,
            offset: 0,
        }).then(response=>{
            let flows = response.data.items || [];
            this.setState({flows: flows, selectedClient: selectedClient});
        });
    }

    state = {
        loading: false,
        clients: [],
        flows: [],
        selectedClient: null,
    }

    renderClients() {
        const selectRow = {
            mode: "checkbox",
            clickToSelect: true,
            hideSelectColumn: true,
            classes: "row-selected",
            onSelect: this.fetchFlows,
        };

        let columns = getClientColumns();
        columns[1] = {dataField: "client_id", text: "Client ID"};

        return (
            <>
              <VeloClientSearch setSearch={this.setSearch}/>
              <div className="no-margins selectable">
                <BootstrapTable
                  hover
                  condensed
                  keyField="client_id"
                  bootstrap4
                  headerClasses="alert alert-secondary"
                  bodyClasses="fixed-table-body"
                  data={this.state.clients}
                  columns={columns}
                  filter={ filterFactory() }
                  selectRow={ selectRow }
                />
              </div>
            </>
        );
    }

    renderFlows() {
        const selectRow = {
            mode: "checkbox",
            clickToSelect: true,
            hideSelectColumn: true,
            classes: "row-selected",
            onSelect: this.addCellFromFlow,
        };

        let columns = getFlowColumns(this.state.selectedClient.client_id);
        return (
            <>
              <div className="no-margins selectable">
                <BootstrapTable
                  hover
                  condensed
                  keyField="session_id"
                  bootstrap4
                  headerClasses="alert alert-secondary"
                  bodyClasses="fixed-table-body"
                  data={this.state.flows}
                  columns={columns}
                  filter={ filterFactory() }
                  selectRow={ selectRow }
                />
              </div>
            </>
        );
    }

    render() {
        return (
            <Modal show={true}
                   size="lg"
                   className="full-height"
                   dialogClassName="modal-90w"
                   onHide={this.props.closeDialog} >
              <Modal.Header closeButton>
                <Modal.Title>Add cell from Hunt</Modal.Title>
              </Modal.Header>
              <Modal.Body>
                {!this.state.selectedClient ?
                 this.renderClients() : this.renderFlows()}
              </Modal.Body>
              <Modal.Footer>
                <Button variant="secondary"
                        onClick={this.props.closeDialog}>
                  Cancel
                </Button>
                <Button variant="primary"
                        onClick={this.addCellFromHunt}>
                  Submit
                </Button>
              </Modal.Footer>
            </Modal>
        );
    }
}
