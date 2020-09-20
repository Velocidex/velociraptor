import "./table.css";

import 'react-bootstrap-table2-paginator/dist/react-bootstrap-table2-paginator.min.css';
import ToolkitProvider, { ColumnToggle } from 'react-bootstrap-table2-toolkit';

import React, { Component } from 'react';
import PropTypes from 'prop-types';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';

import BootstrapTable from 'react-bootstrap-table-next';
import paginationFactory from 'react-bootstrap-table2-paginator';
import Dropdown from 'react-bootstrap/Dropdown';
import Button from 'react-bootstrap/Button';
import Modal from 'react-bootstrap/Modal';

import VeloNotImplemented from '../core/notimplemented.js';
import VeloAce from '../core/ace.js';

const { ToggleList } = ColumnToggle;


// Shows the InspectRawJson modal dialog UI.
class InspectRawJson extends Component {
    static propTypes = {
        rows: PropTypes.array,
    }

    state = {
        show: false,
    }

    update(value) {
        console.log(value);
    }

    render() {
        let rows = [];

        for(var i=0;i<this.props.rows.length;i++) {
            let copy = Object.assign({}, this.props.rows[i]);
            delete copy["_id"];
            rows.push(copy);
        }

        let serialized = JSON.stringify(rows, null, 2);

        return (
            <>
              <Button variant="default"
                      onClick={() => this.setState({show: true})} >
                <FontAwesomeIcon icon="binoculars"/>
              </Button>
              <Modal show={this.state.show}
                     dialogClassName="modal-90w"
                     onHide={(e) => this.setState({show: false})}>
                <Modal.Header closeButton>
                  <Modal.Title>Raw Response JSON</Modal.Title>
                </Modal.Header>

                <Modal.Body>
                  <VeloAce text={serialized}
                    onChange={(value) => this.update(value) }
                  />
                </Modal.Body>

                <Modal.Footer>
                  <Button variant="secondary"
                          onClick={() => this.setState({show: false})} >
                    Close
                  </Button>
                </Modal.Footer>
              </Modal>
</>
        );
    };
}

// Toggle columns on or off - helps when the table is very wide.
const ColumnToggleList = (e) => {
    const { columns, onColumnToggle, toggles } = e;
    let buttons = columns.map(column => ({
        ...column,
        toggle: toggles[column.dataField]
    }))
        .map((column, index) => (
            <Dropdown.Item
              eventKey={index}
              type="button"
              key={ column.dataField }
              className={ `btn btn-default ${column.toggle ? 'active' : ''}` }
              data-toggle="button"
              aria-pressed={ column.toggle ? 'true' : 'false' }
              onClick={ () => onColumnToggle(column.dataField) }
            >
              { column.text }
            </Dropdown.Item>
        ));

    return (
        <>
          <Dropdown>
            <Dropdown.Toggle variant="default" id="dropdown-basic">
              <FontAwesomeIcon icon="columns"/>
            </Dropdown.Toggle>

            <Dropdown.Menu>
              { buttons }
            </Dropdown.Menu>
          </Dropdown>
        </>
    );
};

const sizePerPageRenderer = ({
  options,
  currSizePerPage,
  onSizePerPageChange
}) => (
  <div className="btn-group" role="group">
    {
      options.map((option) => {
        const isSelect = currSizePerPage === `${option.page}`;
        return (
          <button
            key={ option.text }
            type="button"
            onClick={ () => onSizePerPageChange(option.page) }
            className={ `btn ${isSelect ? 'btn-secondary' : 'btn-default'}` }
          >
            { option.text }
          </button>
        );
      })
    }
  </div>
);

class VeloTable extends Component {
    static propTypes = {
        rows: PropTypes.array,
        columns: PropTypes.array,
    }

    state = {
        download: false,
    }

    set = (k, v, e) => {
        let new_state  = Object.assign({}, this.state);
        new_state[k] = v;
        this.setState(new_state);
    }

    render() {
        if (!this.props.rows || !this.props.columns) {
            return <div></div>;
        }

        let rows = this.props.rows;

        let columns = [{dataField: '_id', hidden: true}];
        for(var i=0;i<this.props.columns.length;i++) {
            var name = this.props.columns[i];
            columns.push({ dataField: name, text: name});
        }

        // Add an id field for react ordering.
        for (var j=0; j<rows.length; j++) {
            rows[j]["_id"] = j;
        }

        return (
            <div className="velo-table">
              <ToolkitProvider
                bootstrap4
                keyField="_id"
                data={ rows }
                columns={ columns }
                columnToggle
            >
            {
                props => (
                    <div>
                      <VeloNotImplemented
                        show={this.state.download}
                        resolve={() => this.set("download", false)}
                      />

                      <div className="row">
                        <div className="btn-group float-left" data-toggle="buttons">
                          <ColumnToggleList { ...props.columnToggleProps } />
                          <InspectRawJson rows={this.props.rows} />
                          <Button variant="default"
                                  onClick={() => this.set("download", true)} >
                            <FontAwesomeIcon icon="download"/>
                          </Button>
                        </div>
                      </div>
                      <div className="row">
                        <BootstrapTable
                                     { ...props.baseProps }
                          hover
                          condensed
                          keyField="_id"
                          headerClasses="alert alert-secondary"
                          bodyClasses="fixed-table-body"
                          pagination={ paginationFactory({
                              showTotal: true,
                              sizePerPageRenderer
                          }) }
                        />
                      </div>
                    </div>
                )
            }
              </ToolkitProvider>
            </div>
        );
    }
};

export default VeloTable;
