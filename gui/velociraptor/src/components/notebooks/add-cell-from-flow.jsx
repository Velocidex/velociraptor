import _ from 'lodash';
import React from 'react';
import PropTypes from 'prop-types';

import api from '../core/api-service.jsx';
import Modal from 'react-bootstrap/Modal';
import BootstrapTable from 'react-bootstrap-table-next';
import filterFactory from 'react-bootstrap-table2-filter';
import Button from 'react-bootstrap/Button';
import VeloClientSearch from '../clients/search.jsx';
import T from '../i8n/i8n.jsx';

import { getClientColumns } from '../clients/clients-list.jsx';
import { flowRowRenderer } from '../flows/flows-list.jsx';
import {CancelToken} from 'axios';

import VeloPagedTable from '../core/paged-table.jsx';


export default class AddCellFromFlowDialog extends React.Component {
    static propTypes = {
        closeDialog: PropTypes.func.isRequired,
        addCell: PropTypes.func,
    }

    componentDidMount = () => {
        this.source = CancelToken.source();
        this.setSearch("all");
    }

    componentWillUnmount() {
        this.source.cancel("unmounted");
    }

    setSearch = (query) => {
        api.get('/v1/SearchClients', {
            query: query,
            limit: 50,
        }, this.source.token).then(resp => {
            let items = resp.data && resp.data.items;
            items = items || [];
            this.setState({loading: false, clients: items});
        });
    }

    addCellFromFlow = (flow) => {
        let client_id = this.state.selectedClient;
        var flow_id = flow.FlowId;
        var query = "SELECT * \nFROM source(\n";
        var sources = flow._ArtifactsWithResults;
        query += "    artifact='" + sources[0] + "',\n";
        for (var i=1; i<sources.length; i++) {
            query += "    -- artifact='" + sources[i] + "',\n";
        }
        query += "    client_id='" + client_id + "',\n    flow_id='" +
            flow_id + "', hunt_id='')\nLIMIT 50\n";

        this.props.addCell(query, "VQL");
        this.props.closeDialog();
    }

    fetchFlows = (selectedClient) => {
        this.setState({selectedClient: selectedClient.client_id});
        return;

        let client_id = selectedClient.client_id;
        api.get("v1/GetClientFlows/" + client_id, {
            count: 100,
            offset: 0,
        }, this.source.token).then(response=>{
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

        return <VeloPagedTable
                 url="v1/GetClientFlows"
                 params={{client_id: this.state.selectedClient}}
                 translate_column_headers={true}
                 prevent_transformations={{
                     Mb: true, Rows: true,
                     State: true, "Last Active": true}}
                 selectRow={selectRow}
                 renderers={flowRowRenderer}
                 version={this.state.version}
                 no_spinner={true}
                 transform={this.state.transform}
                 setTransform={x=>{
                     this.setState({transform: x});
                 }}
                 no_toolbar={true}
               />;
    }

    render() {
        return (
            <Modal show={true}
                   size="lg"
                   className="full-height"
                   dialogClassName="modal-90w"
                   onHide={this.props.closeDialog} >
              <Modal.Header closeButton>
                <Modal.Title>{T("Add cell from Flow")}</Modal.Title>
              </Modal.Header>
              <Modal.Body>
                {!this.state.selectedClient ?
                 this.renderClients() : this.renderFlows()}
              </Modal.Body>
              <Modal.Footer>
                <Button variant="secondary"
                        onClick={this.props.closeDialog}>
                  {T("Cancel")}
                </Button>
                <Button variant="primary"
                        onClick={this.addCellFromHunt}>
                  {T("Submit")}
                </Button>
              </Modal.Footer>
            </Modal>
        );
    }
}
